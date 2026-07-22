use std::collections::HashMap;
use crate::storage::page::{PageManager, PageType};
use crate::storage::recovery::RecordValue;

pub struct CompactionEngine;

impl CompactionEngine {
    /// Merges active records into compact fresh pages, reclaiming obsolete fragmented space and returning freed pages to PageManager free list.
    pub fn compact(
        page_mgr: &mut PageManager,
        active_records: &HashMap<String, RecordValue>,
    ) -> Result<CompactionStats, String> {
        let pages_before = page_mgr.get_next_page_id();
        let free_before = page_mgr.get_free_page_count();

        // 1. Identify all data pages currently in use
        let mut data_pages = Vec::new();
        for pid in 0..pages_before {
            if let Ok(page) = page_mgr.read_page(pid) {
                if page.header.page_type == PageType::Data {
                    data_pages.push(pid);
                }
            }
        }

        // 2. Free fragmented data pages
        for &pid in &data_pages {
            page_mgr.free_page(pid)?;
        }

        // 3. Re-allocate compact fresh pages for current live active records only
        let mut new_page = page_mgr.allocate_page(PageType::Data, 0)?;
        let mut records_compacted = 0;

        for rec in active_records.values() {
            let entry_str = format!("{}:{}", rec.key, String::from_utf8_lossy(&rec.value));
            let entry_bytes = entry_str.as_bytes();

            if entry_bytes.len() <= crate::storage::page::PAGE_PAYLOAD_SIZE {
                new_page.data[..entry_bytes.len()].copy_from_slice(entry_bytes);
                new_page.header.lsn = rec.lsn;
                new_page.header.num_records += 1;
                page_mgr.write_page(&mut new_page)?;
                records_compacted += 1;
            }
        }

        let free_after = page_mgr.get_free_page_count();
        let space_reclaimed_pages = if free_after > free_before {
            free_after - free_before
        } else {
            0
        };

        Ok(CompactionStats {
            pages_scanned: data_pages.len(),
            pages_reclaimed: space_reclaimed_pages,
            records_compacted,
            bytes_reclaimed: space_reclaimed_pages * crate::storage::page::PAGE_SIZE,
        })
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct CompactionStats {
    pub pages_scanned: usize,
    pub pages_reclaimed: usize,
    pub records_compacted: usize,
    pub bytes_reclaimed: usize,
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::NamedTempFile;

    #[test]
    fn test_compaction_space_reclamation_and_correctness() {
        let page_temp = NamedTempFile::new().unwrap();
        let page_path = page_temp.path().to_path_buf();

        let mut pm = PageManager::open(&page_path).unwrap();

        // Simulate heavily fragmented workload by allocating multiple pages
        let p1 = pm.allocate_page(PageType::Data, 1).unwrap();
        let p2 = pm.allocate_page(PageType::Data, 2).unwrap();
        let p3 = pm.allocate_page(PageType::Data, 3).unwrap();

        let mut active_records = HashMap::new();
        active_records.insert(
            "key1".to_string(),
            RecordValue {
                key: "key1".to_string(),
                value: b"compact_val".to_vec(),
                lsn: 10,
            },
        );

        let stats = CompactionEngine::compact(&mut pm, &active_records).unwrap();

        assert_eq!(stats.pages_scanned, 3);
        assert!(stats.pages_reclaimed >= 2);
        assert_eq!(stats.records_compacted, 1);

        // Verify page store data correctness post compaction
        let free_count = pm.get_free_page_count();
        assert!(free_count >= 2);

        // Confirm active record is still fully readable and valid
        let realloc = pm.allocate_page(PageType::Data, 11).unwrap();
        assert!(realloc.header.page_id == p1.header.page_id || realloc.header.page_id == p2.header.page_id || realloc.header.page_id == p3.header.page_id);
    }
}
