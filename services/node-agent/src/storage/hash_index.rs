use std::collections::HashMap;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct RecordLocation {
    pub page_id: u64,
    pub offset: u32,
    pub lsn: u64,
}

#[derive(Debug, Default)]
pub struct HashIndex {
    index: HashMap<String, RecordLocation>,
}

impl HashIndex {
    pub fn new() -> Self {
        HashIndex {
            index: HashMap::new(),
        }
    }

    pub fn insert(&mut self, key: String, page_id: u64, offset: u32, lsn: u64) {
        self.index.insert(key, RecordLocation { page_id, offset, lsn });
    }

    pub fn get(&self, key: &str) -> Option<RecordLocation> {
        self.index.get(key).copied()
    }

    pub fn remove(&mut self, key: &str) -> Option<RecordLocation> {
        self.index.remove(key)
    }

    pub fn contains_key(&self, key: &str) -> bool {
        self.index.contains_key(key)
    }

    pub fn len(&self) -> usize {
        self.index.len()
    }

    pub fn is_empty(&self) -> bool {
        self.index.is_empty()
    }

    pub fn clear(&mut self) {
        self.index.clear();
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_hash_index_crud() {
        let mut idx = HashIndex::new();

        idx.insert("key1".to_string(), 1, 0, 100);
        idx.insert("key2".to_string(), 1, 64, 101);

        assert_eq!(idx.len(), 2);
        assert_eq!(idx.get("key1"), Some(RecordLocation { page_id: 1, offset: 0, lsn: 100 }));
        assert_eq!(idx.get("key2"), Some(RecordLocation { page_id: 1, offset: 64, lsn: 101 }));

        idx.insert("key1".to_string(), 2, 0, 105);
        assert_eq!(idx.get("key1"), Some(RecordLocation { page_id: 2, offset: 0, lsn: 105 }));

        let removed = idx.remove("key1");
        assert_eq!(removed, Some(RecordLocation { page_id: 2, offset: 0, lsn: 105 }));
        assert_eq!(idx.get("key1"), None);
        assert_eq!(idx.len(), 1);
    }
}
