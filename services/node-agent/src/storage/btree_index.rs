use std::collections::BTreeMap;
use crate::storage::hash_index::RecordLocation;

pub const BTREE_NODE_CAPACITY: usize = 32;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct BTreeRecord {
    pub key: String,
    pub location: RecordLocation,
}

#[derive(Debug, Default)]
pub struct BTreeIndex {
    tree: BTreeMap<String, RecordLocation>,
}

impl BTreeIndex {
    pub fn new() -> Self {
        BTreeIndex {
            tree: BTreeMap::new(),
        }
    }

    pub fn insert(&mut self, key: String, page_id: u64, offset: u32, lsn: u64) {
        self.tree.insert(key, RecordLocation { page_id, offset, lsn });
    }

    pub fn get(&self, key: &str) -> Option<RecordLocation> {
        self.tree.get(key).copied()
    }

    pub fn remove(&mut self, key: &str) -> Option<RecordLocation> {
        self.tree.remove(key)
    }

    /// Performs an ordered range scan query between start_key (inclusive) and end_key (inclusive).
    pub fn range_scan(&self, start_key: &str, end_key: &str) -> Vec<BTreeRecord> {
        self.tree
            .range(start_key.to_string()..=end_key.to_string())
            .map(|(k, v)| BTreeRecord {
                key: k.clone(),
                location: *v,
            })
            .collect()
    }

    pub fn len(&self) -> usize {
        self.tree.len()
    }

    pub fn is_empty(&self) -> bool {
        self.tree.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::storage::hash_index::HashIndex;

    #[test]
    fn test_btree_point_lookup_and_range_scan() {
        let mut btree = BTreeIndex::new();
        let mut hash_idx = HashIndex::new();

        let items = vec![
            ("customer:100", 1, 0, 10),
            ("customer:200", 1, 64, 11),
            ("customer:150", 2, 0, 12),
            ("customer:300", 2, 64, 13),
            ("customer:050", 3, 0, 14),
        ];

        for (k, pid, off, lsn) in &items {
            btree.insert(k.to_string(), *pid, *off, *lsn);
            hash_idx.insert(k.to_string(), *pid, *off, *lsn);
        }

        // Cross-check point lookups between B+Tree and Hash Index
        for (k, _, _, _) in &items {
            let btree_res = btree.get(k);
            let hash_res = hash_idx.get(k);
            assert_eq!(btree_res, hash_res);
        }

        // Test ordered range scan
        let scan_results = btree.range_scan("customer:100", "customer:200");
        assert_eq!(scan_results.len(), 3);
        assert_eq!(scan_results[0].key, "customer:100");
        assert_eq!(scan_results[1].key, "customer:150");
        assert_eq!(scan_results[2].key, "customer:200");
    }
}
