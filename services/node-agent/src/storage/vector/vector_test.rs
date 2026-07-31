#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use tempfile::tempdir;

    use crate::storage::engine::StorageEngine;
    use crate::storage::replication::ReplicationRole;

    #[test]
    fn test_step1_data_model_wal_durability() {
        let dir = tempdir().unwrap();
        let db_dir = dir.path();

        let lsn;
        {
            let mut engine = StorageEngine::open("vec_db", db_dir, ReplicationRole::Leader).unwrap();
            let mut meta = HashMap::new();
            meta.insert("region".to_string(), "india".to_string());

            lsn = engine
                .insert_vector(
                    "doc-1".to_string(),
                    b"first vector payload".to_vec(),
                    vec![1.0, 0.0, 0.0],
                    meta,
                )
                .unwrap();
            assert!(lsn > 0);
        }

        // Restart engine post-"shutdown" to verify WAL recovery of vector record
        let engine_recovered = StorageEngine::open("vec_db", db_dir, ReplicationRole::Leader).unwrap();
        assert_eq!(engine_recovered.vector_records.len(), 1);
        let vrec = engine_recovered.vector_records.get("doc-1").unwrap();
        assert_eq!(vrec.embedding, vec![1.0, 0.0, 0.0]);
        assert_eq!(vrec.metadata.get("region").unwrap(), "india");
    }

    #[test]
    fn test_step2_and_3_exact_search_mathematical_correctness() {
        let dir = tempdir().unwrap();
        let mut engine = StorageEngine::open("exact_db", dir.path(), ReplicationRole::Leader).unwrap();

        // Hand-checkable dataset:
        // doc-A: [1, 0] -> cosine sim with [1, 0] is 1.0
        // doc-B: [0.7071, 0.7071] -> cosine sim with [1, 0] is ~0.7071
        // doc-C: [0, 1] -> cosine sim with [1, 0] is 0.0
        // doc-D: [-1, 0] -> cosine sim with [1, 0] is -1.0

        engine.insert_vector("doc-A".to_string(), vec![], vec![1.0, 0.0], HashMap::new()).unwrap();
        engine.insert_vector("doc-B".to_string(), vec![], vec![std::f32::consts::FRAC_1_SQRT_2, std::f32::consts::FRAC_1_SQRT_2], HashMap::new()).unwrap();
        engine.insert_vector("doc-C".to_string(), vec![], vec![0.0, 1.0], HashMap::new()).unwrap();
        engine.insert_vector("doc-D".to_string(), vec![], vec![-1.0, 0.0], HashMap::new()).unwrap();

        let query = vec![1.0, 0.0];
        let results = engine.search_vector(&query, 4, "", true);

        assert_eq!(results.len(), 4);
        assert_eq!(results[0].id, "doc-A");
        assert!((results[0].similarity - 1.0).abs() < 1e-4);

        assert_eq!(results[1].id, "doc-B");
        assert!((results[1].similarity - std::f32::consts::FRAC_1_SQRT_2).abs() < 1e-3);

        assert_eq!(results[2].id, "doc-C");
        assert!((results[2].similarity - 0.0).abs() < 1e-4);

        assert_eq!(results[3].id, "doc-D");
        assert!((results[3].similarity - (-1.0)).abs() < 1e-4);
    }

    #[test]
    fn test_step4_metadata_filtered_search_zero_leakage() {
        let dir = tempdir().unwrap();
        let mut engine = StorageEngine::open("filter_db", dir.path(), ReplicationRole::Leader).unwrap();

        let mut meta1 = HashMap::new();
        meta1.insert("region".to_string(), "india".to_string());
        meta1.insert("category".to_string(), "invoice".to_string());

        let mut meta2 = HashMap::new();
        meta2.insert("region".to_string(), "us-east".to_string());
        meta2.insert("category".to_string(), "invoice".to_string());

        let mut meta3 = HashMap::new();
        meta3.insert("region".to_string(), "india".to_string());
        meta3.insert("category".to_string(), "contract".to_string());

        engine.insert_vector("doc-in-inv".to_string(), vec![], vec![0.9, 0.1], meta1).unwrap();
        engine.insert_vector("doc-us-inv".to_string(), vec![], vec![0.95, 0.05], meta2).unwrap();
        engine.insert_vector("doc-in-ctr".to_string(), vec![], vec![0.99, 0.01], meta3).unwrap();

        let query = vec![1.0, 0.0];

        // Filter: region = 'india' AND category = 'invoice'
        let results = engine.search_vector(&query, 10, "region = 'india' AND category = 'invoice'", true);

        assert_eq!(results.len(), 1);
        assert_eq!(results[0].id, "doc-in-inv");
    }

    #[test]
    fn test_step5_hybrid_search_btree_plus_vector() {
        let dir = tempdir().unwrap();
        let mut engine = StorageEngine::open("hybrid_db", dir.path(), ReplicationRole::Leader).unwrap();

        // Populate B+Tree & Vector store with key prefix vec:doc-00 to vec:doc-99
        for i in 0..20 {
            let id = format!("doc-{:02}", i);
            let mut meta = HashMap::new();
            meta.insert("idx".to_string(), i.to_string());
            let emb = vec![i as f32 / 20.0, 1.0 - (i as f32 / 20.0)];
            engine.insert_vector(id, vec![], emb, meta).unwrap();
        }

        let query = vec![0.5, 0.5];

        // Hybrid range scan on key range vec:doc-05 to vec:doc-10
        let results = engine.hybrid_search(&query, "vec:doc-05", "vec:doc-10", 10);

        assert!(!results.is_empty());
        for res in &results {
            assert!(res.id.as_str() >= "doc-05" && res.id.as_str() <= "doc-10");
        }
    }

    #[test]
    fn test_step6_hnsw_ann_recall_benchmark() {
        let dir = tempdir().unwrap();
        let mut engine = StorageEngine::open("ann_bench_db", dir.path(), ReplicationRole::Leader).unwrap();

        // Populate dataset with 500 vectors for recall evaluation
        let num_vectors = 500;
        let dim = 16;

        for i in 0..num_vectors {
            let id = format!("vector-{}", i);
            let mut emb = Vec::with_capacity(dim);
            for d in 0..dim {
                emb.push(((i * (d + 1)) % 100) as f32 / 100.0);
            }
            engine.insert_vector(id, vec![], emb, HashMap::new()).unwrap();
        }

        let query_vec = vec![0.5f32; dim];
        let top_k = 10;

        // Ground truth exact search
        let exact_results = engine.search_vector(&query_vec, top_k, "", true);
        let exact_ids: std::collections::HashSet<_> = exact_results.iter().map(|r| &r.id).collect();

        // HNSW ANN search
        let ann_results = engine.search_vector(&query_vec, top_k, "", false);
        let ann_ids: std::collections::HashSet<_> = ann_results.iter().map(|r| &r.id).collect();

        // Calculate recall@10
        let matches = exact_ids.intersection(&ann_ids).count();
        let recall_k = matches as f32 / top_k as f32;

        println!("Measured HNSW Recall@10: {}/{} ({:.2}%)", matches, top_k, recall_k * 100.0);
        assert!(recall_k >= 0.80, "HNSW recall@10 must be >= 80%, got {:.2}%", recall_k * 100.0);
    }

    #[test]
    fn test_step7_crash_consistency_for_vector_inserts() {
        let dir = tempdir().unwrap();
        let db_dir = dir.path();

        let num_vectors = 25;
        {
            let mut engine = StorageEngine::open("crash_vec_db", db_dir, ReplicationRole::Leader).unwrap();
            for i in 0..num_vectors {
                let id = format!("crash-vec-{}", i);
                let mut meta = HashMap::new();
                meta.insert("tag".to_string(), "crash-test".to_string());
                engine
                    .insert_vector(
                        id,
                        b"crash test payload".to_vec(),
                        vec![i as f32, (i * 2) as f32],
                        meta,
                    )
                    .unwrap();
            }
            // Abrupt termination without graceful shutdown
        }

        // Restart storage engine post abrupt kill
        let engine_recovered = StorageEngine::open("crash_vec_db", db_dir, ReplicationRole::Leader).unwrap();

        assert_eq!(engine_recovered.vector_records.len(), num_vectors);
        assert_eq!(engine_recovered.hnsw_idx.len(), num_vectors);

        for i in 0..num_vectors {
            let id = format!("crash-vec-{}", i);
            let vrec = engine_recovered.vector_records.get(&id).expect("Vector missing post crash recovery");
            assert_eq!(vrec.embedding, vec![i as f32, (i * 2) as f32]);
        }
    }
}
