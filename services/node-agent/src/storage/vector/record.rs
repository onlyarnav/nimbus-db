use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct VectorRecord {
    pub id: String,
    pub data: Vec<u8>,
    pub embedding: Vec<f32>,
    pub metadata: HashMap<String, String>,
}

impl VectorRecord {
    pub fn new(
        id: String,
        data: Vec<u8>,
        embedding: Vec<f32>,
        metadata: HashMap<String, String>,
    ) -> Self {
        VectorRecord {
            id,
            data,
            embedding,
            metadata,
        }
    }

    pub fn to_bytes(&self) -> Result<Vec<u8>, String> {
        serde_json::to_vec(self).map_err(|e| format!("Failed to serialize VectorRecord: {}", e))
    }

    pub fn from_bytes(bytes: &[u8]) -> Result<Self, String> {
        serde_json::from_slice(bytes).map_err(|e| format!("Failed to deserialize VectorRecord: {}", e))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_vector_record_serde_roundtrip() {
        let mut meta = HashMap::new();
        meta.insert("region".to_string(), "india".to_string());
        meta.insert("category".to_string(), "invoice".to_string());

        let record = VectorRecord::new(
            "vec:100".to_string(),
            b"test payload".to_vec(),
            vec![0.1, 0.2, 0.3, 0.4],
            meta,
        );

        let bytes = record.to_bytes().unwrap();
        let recovered = VectorRecord::from_bytes(&bytes).unwrap();

        assert_eq!(record, recovered);
        assert_eq!(recovered.metadata.get("region").unwrap(), "india");
    }
}
