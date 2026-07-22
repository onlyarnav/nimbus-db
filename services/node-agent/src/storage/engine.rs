use std::collections::HashMap;
use std::path::{Path, PathBuf};
use crate::storage::btree_index::{BTreeIndex, BTreeRecord};
use crate::storage::compaction::{CompactionEngine, CompactionStats};
use crate::storage::hash_index::{HashIndex, RecordLocation};
use crate::storage::page::{PageManager, PageType};
use crate::storage::recovery::{RecordValue, RecoveryEngine};
use crate::storage::replication::{ReplicationEngine, ReplicationRole};
use crate::storage::snapshot::SnapshotManager;
use crate::storage::wal::{OpType, SyncPolicy, WalManager};

pub struct StorageEngine {
    db_id: String,
    data_dir: PathBuf,
    pub page_mgr: PageManager,
    pub wal: WalManager,
    pub hash_idx: HashIndex,
    pub btree_idx: BTreeIndex,
    pub replication: ReplicationEngine,
    pub active_records: HashMap<String, RecordValue>,
}

impl StorageEngine {
    pub fn open<P: AsRef<Path>>(
        database_id: &str,
        data_dir: P,
        role: ReplicationRole,
    ) -> Result<Self, String> {
        let dir = data_dir.as_ref();
        std::fs::create_dir_all(dir).map_err(|e| e.to_string())?;

        let page_file = dir.join(format!("{}.pages", database_id));
        let wal_file = dir.join(format!("{}.wal", database_id));

        let mut page_mgr = PageManager::open(&page_file)?;
        let mut wal = WalManager::open(&wal_file, SyncPolicy::SyncPerWrite)?;

        // Startup Crash Recovery
        let active_records = RecoveryEngine::recover(&mut wal, &mut page_mgr, 0)?;

        let mut hash_idx = HashIndex::new();
        let mut btree_idx = BTreeIndex::new();

        // Rebuild indexes from recovered state
        for (k, v) in &active_records {
            hash_idx.insert(k.clone(), 0, 0, v.lsn);
            btree_idx.insert(k.clone(), 0, 0, v.lsn);
        }

        let replication = ReplicationEngine::new(role, 3000);

        Ok(StorageEngine {
            db_id: database_id.to_string(),
            data_dir: dir.to_path_buf(),
            page_mgr,
            wal,
            hash_idx,
            btree_idx,
            replication,
            active_records,
        })
    }

    pub fn put(&mut self, key: String, value: Vec<u8>) -> Result<u64, String> {
        let op = if self.active_records.contains_key(&key) {
            OpType::Update
        } else {
            OpType::Insert
        };

        // 1. Append to WAL (durable before ack)
        let lsn = self.wal.append(op, key.clone(), value.clone())?;

        // 2. Allocate / Write to 4KB page
        let mut page = self.page_mgr.allocate_page(PageType::Data, lsn)?;
        let entry_str = format!("{}:{}", key, String::from_utf8_lossy(&value));
        let entry_bytes = entry_str.as_bytes();
        if entry_bytes.len() <= crate::storage::page::PAGE_PAYLOAD_SIZE {
            page.data[..entry_bytes.len()].copy_from_slice(entry_bytes);
            page.header.num_records += 1;
            self.page_mgr.write_page(&mut page)?;
        }

        let loc = RecordLocation {
            page_id: page.header.page_id,
            offset: 0,
            lsn,
        };

        // 3. Update Indexes and Memtable
        self.hash_idx.insert(key.clone(), loc.page_id, loc.offset, lsn);
        self.btree_idx.insert(key.clone(), loc.page_id, loc.offset, lsn);
        self.active_records.insert(
            key.clone(),
            RecordValue {
                key,
                value,
                lsn,
            },
        );

        Ok(lsn)
    }

    pub fn get(&self, key: &str) -> Option<Vec<u8>> {
        self.active_records.get(key).map(|r| r.value.clone())
    }

    pub fn delete(&mut self, key: &str) -> Result<bool, String> {
        if !self.active_records.contains_key(key) {
            return Ok(false);
        }

        let _lsn = self.wal.append(OpType::Delete, key.to_string(), Vec::new())?;
        self.hash_idx.remove(key);
        self.btree_idx.remove(key);
        self.active_records.remove(key);

        Ok(true)
    }

    pub fn range_scan(&self, start_key: &str, end_key: &str) -> Vec<BTreeRecord> {
        self.btree_idx.range_scan(start_key, end_key)
    }

    pub fn backup(&mut self) -> Result<String, String> {
        let backup_dir = self.data_dir.join("backups");
        let lsn = self.wal.get_next_lsn() - 1;
        SnapshotManager::create_snapshot(&backup_dir, &self.db_id, lsn, &self.active_records)
    }

    pub fn restore(&mut self, snapshot_path: &str) -> Result<(), String> {
        let snap_data = SnapshotManager::load_snapshot(snapshot_path)?;
        self.active_records.clear();
        self.hash_idx.clear();
        self.btree_idx = BTreeIndex::new();

        for (k, (val, _lsn)) in snap_data.records {
            self.put(k, val)?;
        }

        Ok(())
    }

    pub fn compact(&mut self) -> Result<CompactionStats, String> {
        CompactionEngine::compact(&mut self.page_mgr, &self.active_records)
    }

    pub fn len(&self) -> usize {
        self.active_records.len()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn test_storage_engine_full_e2e_flow() {
        let dir = tempdir().unwrap();
        let db_dir = dir.path();

        let mut engine = StorageEngine::open("test_db", db_dir, ReplicationRole::Leader).unwrap();

        // 1. Put & Get
        engine.put("user:1".to_string(), b"Alice".to_vec()).unwrap();
        engine.put("user:2".to_string(), b"Bob".to_vec()).unwrap();

        assert_eq!(engine.get("user:1"), Some(b"Alice".to_vec()));
        assert_eq!(engine.get("user:2"), Some(b"Bob".to_vec()));

        // 2. Backup & Restore E2E Test
        let backup_path = engine.backup().unwrap();
        engine.delete("user:1").unwrap();
        assert_eq!(engine.get("user:1"), None);

        engine.restore(&backup_path).unwrap();
        assert_eq!(engine.get("user:1"), Some(b"Alice".to_vec()));

        // 3. Range Scan
        let scan = engine.range_scan("user:1", "user:2");
        assert_eq!(scan.len(), 2);
    }
}
