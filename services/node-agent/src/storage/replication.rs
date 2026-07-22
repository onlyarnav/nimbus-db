use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};
use crate::storage::wal::{LogRecord, WalManager};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ReplicationRole {
    Leader,
    Follower,
}

#[derive(Debug, Clone)]
pub struct ReplicationAck {
    pub follower_id: String,
    pub lsn: u64,
    pub timestamp: Instant,
}

pub struct ReplicationEngine {
    role: ReplicationRole,
    followers: Arc<Mutex<HashMap<String, u64>>>, // follower_id -> last_acked_lsn
    ack_timeout: Duration,
}

impl ReplicationEngine {
    pub fn new(role: ReplicationRole, ack_timeout_ms: u64) -> Self {
        ReplicationEngine {
            role,
            followers: Arc::new(Mutex::new(HashMap::new())),
            ack_timeout: Duration::from_millis(ack_timeout_ms),
        }
    }

    pub fn register_follower(&self, follower_id: String) {
        let mut followers = self.followers.lock().unwrap();
        followers.insert(follower_id, 0);
    }

    pub fn remove_follower(&self, follower_id: &str) {
        let mut followers = self.followers.lock().unwrap();
        followers.remove(follower_id);
    }

    pub fn process_follower_ack(&self, follower_id: &str, lsn: u64) {
        let mut followers = self.followers.lock().unwrap();
        if let Some(entry) = followers.get_mut(follower_id) {
            if lsn > *entry {
                *entry = lsn;
            }
        }
    }

    /// Replicates WAL record from Leader to Followers with ACK quorum gating & timeout fallback to degraded mode.
    pub fn replicate_record(
        &self,
        record: &LogRecord,
        followers_state: &Arc<Mutex<HashMap<String, Vec<LogRecord>>>>,
    ) -> Result<ReplicationSummary, String> {
        if self.role != ReplicationRole::Leader {
            return Err("Only leader nodes can originate replication streams".to_string());
        }

        let start_time = Instant::now();

        // 1. Push record to all registered follower channels
        let followers = self.followers.lock().unwrap().clone();
        if followers.is_empty() {
            return Ok(ReplicationSummary {
                acked_followers: 0,
                total_followers: 0,
                degraded_mode: false,
                replication_lag_ms: start_time.elapsed().as_secs_f64() * 1000.0,
            });
        }

        let mut channel_map = followers_state.lock().unwrap();
        for follower_id in followers.keys() {
            let stream = channel_map.entry(follower_id.clone()).or_insert_with(Vec::new);
            stream.push(record.clone());
        }
        drop(channel_map);

        // 2. Wait for ACKs with timeout (ACK Quorum gating)
        let deadline = Instant::now() + self.ack_timeout;
        let mut acked_count = 0;

        while Instant::now() < deadline {
            let current_acks = self.followers.lock().unwrap();
            acked_count = current_acks.values().filter(|&&l| l >= record.lsn).count();
            if acked_count == followers.len() {
                break;
            }
            std::thread::sleep(Duration::from_millis(5));
        }

        let degraded = acked_count < followers.len();

        Ok(ReplicationSummary {
            acked_followers: acked_count,
            total_followers: followers.len(),
            degraded_mode: degraded,
            replication_lag_ms: start_time.elapsed().as_secs_f64() * 1000.0,
        })
    }

    /// Follower side: receives replicated log record from leader stream and appends to follower WAL.
    pub fn apply_replicated_record(
        follower_wal: &mut WalManager,
        record: &LogRecord,
    ) -> Result<u64, String> {
        follower_wal.append(record.op_type, record.key.clone(), record.value.clone())
    }
}

#[derive(Debug, Clone)]
pub struct ReplicationSummary {
    pub acked_followers: usize,
    pub total_followers: usize,
    pub degraded_mode: bool,
    pub replication_lag_ms: f64,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::storage::wal::{OpType, SyncPolicy};
    use tempfile::NamedTempFile;

    #[test]
    fn test_replication_leader_follower_convergence() {
        let leader_engine = ReplicationEngine::new(ReplicationRole::Leader, 1000);
        leader_engine.register_follower("node-2".to_string());
        leader_engine.register_follower("node-3".to_string());

        let follower_channels = Arc::new(Mutex::new(HashMap::new()));
        let record = LogRecord::new(1, OpType::Insert, "user:100".to_string(), b"Active".to_vec());

        // Simulate follower ACK background worker
        let engine_ref = Arc::new(leader_engine);
        let engine_clone = Arc::clone(&engine_ref);
        std::thread::spawn(move || {
            std::thread::sleep(Duration::from_millis(20));
            engine_clone.process_follower_ack("node-2", 1);
            engine_clone.process_follower_ack("node-3", 1);
        });

        let summary = engine_ref.replicate_record(&record, &follower_channels).unwrap();

        assert_eq!(summary.acked_followers, 2);
        assert_eq!(summary.total_followers, 2);
        assert!(!summary.degraded_mode);
        assert!(summary.replication_lag_ms >= 0.0);

        // Verify follower WAL convergence
        let f_temp = NamedTempFile::new().unwrap();
        let mut follower_wal = WalManager::open(f_temp.path(), SyncPolicy::SyncPerWrite).unwrap();
        ReplicationEngine::apply_replicated_record(&mut follower_wal, &record).unwrap();

        let f_recs = follower_wal.read_all().unwrap();
        assert_eq!(f_recs.len(), 1);
        assert_eq!(f_recs[0].key, "user:100");
    }

    #[test]
    fn test_replication_follower_failure_degraded_mode() {
        let leader_engine = ReplicationEngine::new(ReplicationRole::Leader, 50); // short 50ms deadline
        leader_engine.register_follower("dead-node".to_string());

        let follower_channels = Arc::new(Mutex::new(HashMap::new()));
        let record = LogRecord::new(2, OpType::Insert, "user:200".to_string(), b"FailTest".to_vec());

        // Dead node sends no ACK -> leader must timeout cleanly after 50ms and report degraded mode without hanging
        let summary = leader_engine.replicate_record(&record, &follower_channels).unwrap();

        assert_eq!(summary.acked_followers, 0);
        assert_eq!(summary.total_followers, 1);
        assert!(summary.degraded_mode);
    }
}
