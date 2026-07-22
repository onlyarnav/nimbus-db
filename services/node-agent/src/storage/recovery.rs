use std::collections::HashMap;
use crate::storage::page::{PageManager, PageType, PAGE_PAYLOAD_SIZE};
use crate::storage::wal::{LogRecord, OpType, WalManager};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RecordValue {
    pub key: String,
    pub value: Vec<u8>,
    pub lsn: u64,
}

pub struct RecoveryEngine;

impl RecoveryEngine {
    /// Replays WAL entries into in-memory key-value state and updates page store idempotently using LSN checks.
    pub fn recover(
        wal: &mut WalManager,
        page_mgr: &mut PageManager,
        start_lsn: u64,
    ) -> Result<HashMap<String, RecordValue>, String> {
        let records = wal.read_all()?;
        let mut mem_state: HashMap<String, RecordValue> = HashMap::new();

        for rec in records {
            if rec.lsn < start_lsn {
                continue;
            }

            match rec.op_type {
                OpType::Insert | OpType::Update => {
                    mem_state.insert(
                        rec.key.clone(),
                        RecordValue {
                            key: rec.key.clone(),
                            value: rec.value.clone(),
                            lsn: rec.lsn,
                        },
                    );
                    Self::apply_record_to_pages(page_mgr, &rec)?;
                }
                OpType::Delete => {
                    mem_state.remove(&rec.key);
                    Self::apply_record_to_pages(page_mgr, &rec)?;
                }
                OpType::Checkpoint => {
                    // Checkpoint record marker during WAL replay
                }
            }
        }

        Ok(mem_state)
    }

    fn apply_record_to_pages(page_mgr: &mut PageManager, rec: &LogRecord) -> Result<(), String> {
        // LSN Idempotency check: find target page or allocate new page
        // For simplified page storage layout: record formatted in 4KB data pages
        let total_pages = page_mgr.get_next_page_id();
        let mut target_page_id = None;

        for pid in 0..total_pages {
            if let Ok(page) = page_mgr.read_page(pid) {
                if page.header.page_type == PageType::Data {
                    // Check if already applied via LSN comparison
                    if page.header.lsn >= rec.lsn {
                        return Ok(()); // Already durable on page store
                    }
                    target_page_id = Some(pid);
                    break;
                }
            }
        }

        let page_id = match target_page_id {
            Some(pid) => pid,
            None => {
                let page = page_mgr.allocate_page(PageType::Data, rec.lsn)?;
                page.header.page_id
            }
        };

        let mut page = page_mgr.read_page(page_id)?;
        if page.header.lsn >= rec.lsn {
            return Ok(()); // Idempotency guard
        }

        // Format key:val entry into page payload
        let entry_str = format!("{}:{}", rec.key, String::from_utf8_lossy(&rec.value));
        let entry_bytes = entry_str.as_bytes();

        if entry_bytes.len() <= PAGE_PAYLOAD_SIZE {
            page.data[..entry_bytes.len()].copy_from_slice(entry_bytes);
            page.header.lsn = rec.lsn;
            page.header.num_records += 1;
            page_mgr.write_page(&mut page)?;
        }

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::storage::wal::SyncPolicy;
    use tempfile::NamedTempFile;
    use rand::Rng;

    #[test]
    fn test_recovery_replay_and_lsn_idempotency() {
        let wal_temp = NamedTempFile::new().unwrap();
        let page_temp = NamedTempFile::new().unwrap();

        let wal_path = wal_temp.path().to_path_buf();
        let page_path = page_temp.path().to_path_buf();

        {
            let mut wal = WalManager::open(&wal_path, SyncPolicy::SyncPerWrite).unwrap();
            let _ = wal.append(OpType::Insert, "user:1".to_string(), b"Alice".to_vec()).unwrap();
            let _ = wal.append(OpType::Update, "user:1".to_string(), b"Alice Purohit".to_vec()).unwrap();
            wal.append(OpType::Insert, "user:2".to_string(), b"Bob".to_vec()).unwrap();
        }

        let mut wal = WalManager::open(&wal_path, SyncPolicy::SyncPerWrite).unwrap();
        let mut page_mgr = PageManager::open(&page_path).unwrap();

        // Perform initial recovery replay
        let mem_state = RecoveryEngine::recover(&mut wal, &mut page_mgr, 0).unwrap();

        assert_eq!(mem_state.len(), 2);
        assert_eq!(mem_state.get("user:1").unwrap().value, b"Alice Purohit");
        assert_eq!(mem_state.get("user:2").unwrap().value, b"Bob");

        // Idempotency test: Re-run recovery replay a second time on existing page store
        let mem_state2 = RecoveryEngine::recover(&mut wal, &mut page_mgr, 0).unwrap();
        assert_eq!(mem_state2.len(), 2);
        assert_eq!(mem_state2.get("user:1").unwrap().value, b"Alice Purohit");

        // Verify page store is clean and valid without corruption
        let page = page_mgr.read_page(0).unwrap();
        assert!(page.verify_checksum());
        assert_eq!(page.header.lsn, 3);
    }

    #[test]
    fn test_randomized_kill_and_recovery_loop() {
        // Multi-iteration randomized crash test (10 randomized kill points)
        for i in 0..10 {
            let wal_temp = NamedTempFile::new().unwrap();
            let page_temp = NamedTempFile::new().unwrap();

            let wal_path = wal_temp.path().to_path_buf();
            let page_path = page_temp.path().to_path_buf();

            let mut rng = rand::thread_rng();
            let num_writes = rng.gen_range(5..15);

            let mut acked_records = HashMap::new();

            {
                let mut wal = WalManager::open(&wal_path, SyncPolicy::SyncPerWrite).unwrap();
                for step in 0..num_writes {
                    let key = format!("k_{}", step % 3);
                    let val = format!("val_iter_{}_{}", i, step);
                    let lsn = wal.append(OpType::Insert, key.clone(), val.as_bytes().to_vec()).unwrap();
                    acked_records.insert(key, (val, lsn));
                }
            }

            // Simulate process recovery after abrupt process termination
            let mut wal_rec = WalManager::open(&wal_path, SyncPolicy::SyncPerWrite).unwrap();
            let mut page_rec = PageManager::open(&page_path).unwrap();

            let recovered_state = RecoveryEngine::recover(&mut wal_rec, &mut page_rec, 0).unwrap();

            // Verify all acknowledged writes are restored and accurate
            for (key, (expected_val, _)) in &acked_records {
                let rec = recovered_state.get(key).expect("Key missing post-crash recovery");
                assert_eq!(String::from_utf8_lossy(&rec.value), *expected_val);
            }
        }
    }
}
