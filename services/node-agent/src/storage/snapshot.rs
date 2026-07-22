use std::collections::HashMap;
use std::fs::{self, File};
use std::io::{Read, Write};
use std::path::Path;
use serde::{Deserialize, Serialize};
use crate::storage::recovery::RecordValue;

#[derive(Debug, Serialize, Deserialize)]
pub struct SnapshotHeader {
    pub database_id: String,
    pub snapshot_id: String,
    pub checkpoint_lsn: u64,
    pub timestamp_unix: u64,
    pub record_count: usize,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct SnapshotData {
    pub header: SnapshotHeader,
    pub records: HashMap<String, (Vec<u8>, u64)>, // key -> (value, lsn)
}

pub struct SnapshotManager;

impl SnapshotManager {
    pub fn create_snapshot<P: AsRef<Path>>(
        backup_dir: P,
        database_id: &str,
        checkpoint_lsn: u64,
        records: &HashMap<String, RecordValue>,
    ) -> Result<String, String> {
        let dir = backup_dir.as_ref();
        fs::create_dir_all(dir).map_err(|e| e.to_string())?;

        let timestamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();

        let snapshot_id = format!("snap_{}_{}", database_id, checkpoint_lsn);
        let snapshot_file_path = dir.join(format!("{}.snap", snapshot_id));

        let mut snap_records = HashMap::new();
        for (k, v) in records {
            snap_records.insert(k.clone(), (v.value.clone(), v.lsn));
        }

        let snap_data = SnapshotData {
            header: SnapshotHeader {
                database_id: database_id.to_string(),
                snapshot_id: snapshot_id.clone(),
                checkpoint_lsn,
                timestamp_unix: timestamp,
                record_count: snap_records.len(),
            },
            records: snap_records,
        };

        let json_bytes = serde_json::to_vec_pretty(&snap_data)
            .map_err(|e| format!("Snapshot serialization failed: {}", e))?;

        let mut file = File::create(&snapshot_file_path).map_err(|e| e.to_string())?;
        file.write_all(&json_bytes).map_err(|e| e.to_string())?;
        file.flush().map_err(|e| e.to_string())?;

        Ok(snapshot_file_path.to_string_lossy().to_string())
    }

    pub fn load_snapshot<P: AsRef<Path>>(snapshot_path: P) -> Result<SnapshotData, String> {
        let mut file = File::open(snapshot_path).map_err(|e| format!("Failed to open snapshot: {}", e))?;
        let mut buf = Vec::new();
        file.read_to_end(&mut buf).map_err(|e| e.to_string())?;

        let snap_data: SnapshotData = serde_json::from_slice(&buf)
            .map_err(|e| format!("Snapshot deserialization failed: {}", e))?;

        Ok(snap_data)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn test_snapshot_create_and_load() {
        let dir = tempdir().unwrap();
        let backup_dir = dir.path();

        let mut records = HashMap::new();
        records.insert(
            "k1".to_string(),
            RecordValue {
                key: "k1".to_string(),
                value: b"v1".to_vec(),
                lsn: 10,
            },
        );

        let snap_path = SnapshotManager::create_snapshot(backup_dir, "db1", 10, &records).unwrap();
        let loaded = SnapshotManager::load_snapshot(&snap_path).unwrap();

        assert_eq!(loaded.header.database_id, "db1");
        assert_eq!(loaded.header.checkpoint_lsn, 10);
        assert_eq!(loaded.header.record_count, 1);
        assert_eq!(loaded.records.get("k1").unwrap().0, b"v1");
    }
}
