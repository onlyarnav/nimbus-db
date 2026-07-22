use std::fs::{File, OpenOptions};
use std::io::{Read, Seek, SeekFrom, Write};
use std::path::{Path, PathBuf};
use byteorder::{BigEndian, ReadBytesExt, WriteBytesExt};
use crc32fast::Hasher;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(u8)]
pub enum OpType {
    Insert = 1,
    Update = 2,
    Delete = 3,
    Checkpoint = 4,
}

impl OpType {
    pub fn from_u8(v: u8) -> Result<Self, String> {
        match v {
            1 => Ok(OpType::Insert),
            2 => Ok(OpType::Update),
            3 => Ok(OpType::Delete),
            4 => Ok(OpType::Checkpoint),
            _ => Err(format!("Invalid OpType: {}", v)),
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct LogRecord {
    pub lsn: u64,
    pub op_type: OpType,
    pub key: String,
    pub value: Vec<u8>,
    pub checksum: u32,
}

impl LogRecord {
    pub fn new(lsn: u64, op_type: OpType, key: String, value: Vec<u8>) -> Self {
        let mut rec = LogRecord {
            lsn,
            op_type,
            key,
            value,
            checksum: 0,
        };
        rec.checksum = rec.calculate_checksum();
        rec
    }

    pub fn calculate_checksum(&self) -> u32 {
        let mut hasher = Hasher::new();
        hasher.update(&self.lsn.to_be_bytes());
        hasher.update(&[self.op_type as u8]);
        hasher.update(&(self.key.len() as u32).to_be_bytes());
        hasher.update(self.key.as_bytes());
        hasher.update(&(self.value.len() as u32).to_be_bytes());
        hasher.update(&self.value);
        hasher.finalize()
    }

    pub fn verify_checksum(&self) -> bool {
        self.checksum == self.calculate_checksum()
    }

    pub fn serialize(&self) -> Vec<u8> {
        let mut buf = Vec::new();
        buf.write_u64::<BigEndian>(self.lsn).unwrap();
        buf.write_u8(self.op_type as u8).unwrap();
        buf.write_u32::<BigEndian>(self.checksum).unwrap();

        let key_bytes = self.key.as_bytes();
        buf.write_u32::<BigEndian>(key_bytes.len() as u32).unwrap();
        buf.extend_from_slice(key_bytes);

        buf.write_u32::<BigEndian>(self.value.len() as u32).unwrap();
        buf.extend_from_slice(&self.value);

        buf
    }

    pub fn deserialize<R: Read>(reader: &mut R) -> Result<Self, String> {
        let lsn = match reader.read_u64::<BigEndian>() {
            Ok(val) => val,
            Err(e) if e.kind() == std::io::ErrorKind::UnexpectedEof => {
                return Err("EOF".to_string());
            }
            Err(e) => return Err(e.to_string()),
        };

        let op_type_u8 = reader.read_u8().map_err(|e| e.to_string())?;
        let op_type = OpType::from_u8(op_type_u8)?;

        let checksum = reader.read_u32::<BigEndian>().map_err(|e| e.to_string())?;

        let key_len = reader.read_u32::<BigEndian>().map_err(|e| e.to_string())? as usize;
        let mut key_buf = vec![0u8; key_len];
        reader.read_exact(&mut key_buf).map_err(|e| e.to_string())?;
        let key = String::from_utf8(key_buf).map_err(|e| e.to_string())?;

        let val_len = reader.read_u32::<BigEndian>().map_err(|e| e.to_string())? as usize;
        let mut val_buf = vec![0u8; val_len];
        reader.read_exact(&mut val_buf).map_err(|e| e.to_string())?;

        let rec = LogRecord {
            lsn,
            op_type,
            key,
            value: val_buf,
            checksum,
        };

        if !rec.verify_checksum() {
            return Err(format!(
                "WAL record checksum mismatch for LSN {}: expected {}, computed {}",
                rec.lsn,
                rec.checksum,
                rec.calculate_checksum()
            ));
        }

        Ok(rec)
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SyncPolicy {
    SyncPerWrite,
    Batched,
}

pub struct WalManager {
    path: PathBuf,
    file: File,
    next_lsn: u64,
    sync_policy: SyncPolicy,
}

impl WalManager {
    pub fn open<P: AsRef<Path>>(path: P, sync_policy: SyncPolicy) -> Result<Self, String> {
        let path_buf = path.as_ref().to_path_buf();
        let mut file = OpenOptions::new()
            .read(true)
            .write(true)
            .create(true)
            .open(&path_buf)
            .map_err(|e| format!("Failed to open WAL file: {}", e))?;

        // Scan WAL to find maximum LSN and clean up trailing torn writes
        file.seek(SeekFrom::Start(0)).map_err(|e| e.to_string())?;
        let mut max_lsn = 0u64;
        let mut valid_bytes = 0u64;

        loop {
            let _current_pos = file.stream_position().map_err(|e| e.to_string())?;
            match LogRecord::deserialize(&mut file) {
                Ok(rec) => {
                    if rec.lsn > max_lsn {
                        max_lsn = rec.lsn;
                    }
                    valid_bytes = file.stream_position().map_err(|e| e.to_string())?;
                }
                Err(err) if err == "EOF" => {
                    break;
                }
                Err(_) => {
                    // Torn/corrupt record at end of WAL - truncate file to last valid boundary
                    file.set_len(valid_bytes).map_err(|e| e.to_string())?;
                    break;
                }
            }
        }

        file.seek(SeekFrom::Start(valid_bytes)).map_err(|e| e.to_string())?;

        Ok(WalManager {
            path: path_buf,
            file,
            next_lsn: max_lsn + 1,
            sync_policy,
        })
    }

    pub fn append(&mut self, op_type: OpType, key: String, value: Vec<u8>) -> Result<u64, String> {
        let lsn = self.next_lsn;
        self.next_lsn += 1;

        let record = LogRecord::new(lsn, op_type, key, value);
        let buf = record.serialize();

        self.file.write_all(&buf).map_err(|e| format!("WAL append failed: {}", e))?;

        if self.sync_policy == SyncPolicy::SyncPerWrite {
            self.file.sync_data().map_err(|e| format!("WAL sync failed: {}", e))?;
        }

        Ok(lsn)
    }

    pub fn flush(&mut self) -> Result<(), String> {
        self.file.sync_data().map_err(|e| format!("WAL sync failed: {}", e))
    }

    pub fn read_all(&mut self) -> Result<Vec<LogRecord>, String> {
        let mut records = Vec::new();
        self.file.seek(SeekFrom::Start(0)).map_err(|e| e.to_string())?;

        loop {
            match LogRecord::deserialize(&mut self.file) {
                Ok(rec) => records.push(rec),
                Err(err) if err == "EOF" => break,
                Err(err) => return Err(format!("WAL read failed: {}", err)),
            }
        }

        Ok(records)
    }

    pub fn get_next_lsn(&self) -> u64 {
        self.next_lsn
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::NamedTempFile;

    #[test]
    fn test_wal_append_and_read_sequential() {
        let temp_file = NamedTempFile::new().unwrap();
        let path = temp_file.path().to_path_buf();

        let mut wal = WalManager::open(&path, SyncPolicy::SyncPerWrite).unwrap();
        let lsn1 = wal.append(OpType::Insert, "key1".to_string(), b"val1".to_vec()).unwrap();
        let lsn2 = wal.append(OpType::Update, "key1".to_string(), b"val2".to_vec()).unwrap();
        let lsn3 = wal.append(OpType::Delete, "key2".to_string(), Vec::new()).unwrap();

        assert_eq!(lsn1, 1);
        assert_eq!(lsn2, 2);
        assert_eq!(lsn3, 3);

        let records = wal.read_all().unwrap();
        assert_eq!(records.len(), 3);
        assert_eq!(records[0].key, "key1");
        assert_eq!(records[0].value, b"val1");
        assert_eq!(records[1].op_type, OpType::Update);
        assert_eq!(records[2].op_type, OpType::Delete);
    }

    #[test]
    fn test_wal_torn_write_truncation() {
        let temp_file = NamedTempFile::new().unwrap();
        let path = temp_file.path().to_path_buf();

        {
            let mut wal = WalManager::open(&path, SyncPolicy::SyncPerWrite).unwrap();
            wal.append(OpType::Insert, "valid1".to_string(), b"data1".to_vec()).unwrap();
            wal.append(OpType::Insert, "valid2".to_string(), b"data2".to_vec()).unwrap();
        }

        // Simulate a torn write by appending partial/garbage bytes to the file
        {
            let mut f = OpenOptions::new().append(true).open(&path).unwrap();
            f.write_all(b"\x00\x00\x00\x00\x00\x00\x00\x03\x01CORRUPT_BYTES").unwrap();
            f.flush().unwrap();
        }

        // Reopening WAL manager must detect torn write, truncate invalid bytes, and recover valid records
        let mut wal_recovered = WalManager::open(&path, SyncPolicy::SyncPerWrite).unwrap();
        let records = wal_recovered.read_all().unwrap();

        assert_eq!(records.len(), 2);
        assert_eq!(records[0].key, "valid1");
        assert_eq!(records[1].key, "valid2");
        assert_eq!(wal_recovered.get_next_lsn(), 3);
    }
}
