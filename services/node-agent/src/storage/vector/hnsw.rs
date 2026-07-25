use std::collections::{HashMap, HashSet};
use std::cmp::Ordering;
use rand::Rng;

use crate::storage::vector::distance::cosine_similarity;

#[derive(Debug, Clone)]
pub struct HnswNode {
    pub id: String,
    pub embedding: Vec<f32>,
    pub level: usize,
    // Layer index -> list of neighbor node IDs
    pub neighbors: Vec<Vec<String>>,
}

pub struct HnswIndex {
    pub max_level: usize,
    pub m: usize,
    pub m_max: usize,
    pub ef_construction: usize,
    pub ef_search: usize,
    pub entry_point: Option<String>,
    pub nodes: HashMap<String, HnswNode>,
    pub ml: f64,
}

#[derive(Debug, Clone)]
pub struct SearchResult {
    pub id: String,
    pub similarity: f32,
}

impl HnswIndex {
    pub fn new(m: usize, ef_construction: usize, ef_search: usize) -> Self {
        let m_max = m;
        let ml = 1.0 / (m as f64).ln();
        HnswIndex {
            max_level: 0,
            m,
            m_max,
            ef_construction,
            ef_search,
            entry_point: None,
            nodes: HashMap::new(),
            ml,
        }
    }

    pub fn default_hnsw() -> Self {
        Self::new(16, 64, 32)
    }

    fn random_level(&self) -> usize {
        let mut rng = rand::thread_rng();
        let r: f64 = rng.gen_range(0.00001..1.0);
        (-r.ln() * self.ml).floor() as usize
    }

    pub fn insert(&mut self, id: String, embedding: Vec<f32>) {
        if self.nodes.contains_key(&id) {
            self.remove(&id);
        }

        let insert_level = self.random_level();
        let mut new_node = HnswNode {
            id: id.clone(),
            embedding: embedding.clone(),
            level: insert_level,
            neighbors: vec![Vec::new(); insert_level + 1],
        };

        if self.entry_point.is_none() {
            self.entry_point = Some(id.clone());
            self.max_level = insert_level;
            self.nodes.insert(id, new_node);
            return;
        }

        let curr_entry = self.entry_point.clone().unwrap();
        let curr_max_level = self.max_level;

        let mut curr_obj = curr_entry;

        // Phase 1: Search top layers down to insert_level + 1
        if insert_level < curr_max_level {
            for level in (insert_level + 1..=curr_max_level).rev() {
                curr_obj = self.search_layer_closest(&embedding, &curr_obj, level);
            }
        }

        // Phase 2: For layers from min(insert_level, curr_max_level) down to 0, search and connect neighbors
        let start_layer = insert_level.min(curr_max_level);
        for level in (0..=start_layer).rev() {
            let candidates = self.search_layer(&embedding, &curr_obj, self.ef_construction, level);
            let neighbors = self.select_neighbors(&candidates, self.m);

            for n_id in &neighbors {
                new_node.neighbors[level].push(n_id.clone());
                
                // Collect candidate embeddings first to avoid simultaneous borrow
                let mut candidate_embs = HashMap::new();
                if let Some(nn) = self.nodes.get(n_id) {
                    if level < nn.neighbors.len() && nn.neighbors[level].len() >= self.m_max {
                        candidate_embs.insert(id.clone(), embedding.clone());
                        for cand_id in &nn.neighbors[level] {
                            if let Some(cand_node) = self.nodes.get(cand_id) {
                                candidate_embs.insert(cand_id.clone(), cand_node.embedding.clone());
                            }
                        }
                    }
                }

                if let Some(neighbor_node) = self.nodes.get_mut(n_id) {
                    if level < neighbor_node.neighbors.len() {
                        neighbor_node.neighbors[level].push(id.clone());
                        // Prune if exceeds m_max
                        if neighbor_node.neighbors[level].len() > self.m_max {
                            let pr_emb = &neighbor_node.embedding;
                            let mut pr_cands = neighbor_node.neighbors[level]
                                .iter()
                                .filter_map(|cand_id| {
                                    candidate_embs.get(cand_id).map(|cand_emb| {
                                        let sim = cosine_similarity(pr_emb, cand_emb);
                                        (cand_id.clone(), sim)
                                    })
                                })
                                .collect::<Vec<_>>();
                            pr_cands.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap());
                            neighbor_node.neighbors[level] = pr_cands
                                .into_iter()
                                .take(self.m_max)
                                .map(|(c_id, _)| c_id)
                                .collect();
                        }
                    }
                }
            }

            if !neighbors.is_empty() {
                curr_obj = neighbors[0].clone();
            }
        }

        self.nodes.insert(id.clone(), new_node);

        if insert_level > self.max_level {
            self.max_level = insert_level;
            self.entry_point = Some(id);
        }
    }

    fn search_layer_closest(&self, query: &[f32], entry: &str, level: usize) -> String {
        let mut best_obj = entry.to_string();
        let mut best_sim = cosine_similarity(query, &self.nodes.get(entry).unwrap().embedding);

        loop {
            let mut changed = false;
            if let Some(node) = self.nodes.get(&best_obj) {
                if level < node.neighbors.len() {
                    for n_id in &node.neighbors[level] {
                        if let Some(n_node) = self.nodes.get(n_id) {
                            let sim = cosine_similarity(query, &n_node.embedding);
                            if sim > best_sim {
                                best_sim = sim;
                                best_obj = n_id.clone();
                                changed = true;
                            }
                        }
                    }
                }
            }
            if !changed {
                break;
            }
        }

        best_obj
    }

    fn search_layer(&self, query: &[f32], entry: &str, ef: usize, level: usize) -> Vec<SearchResult> {
        let mut visited = HashSet::new();
        visited.insert(entry.to_string());

        let entry_sim = cosine_similarity(query, &self.nodes.get(entry).unwrap().embedding);

        let mut candidates = Vec::new();
        candidates.push(SearchResult {
            id: entry.to_string(),
            similarity: entry_sim,
        });

        let mut queue = vec![entry.to_string()];

        while let Some(curr_id) = queue.pop() {
            if let Some(node) = self.nodes.get(&curr_id) {
                if level < node.neighbors.len() {
                    for n_id in &node.neighbors[level] {
                        if !visited.contains(n_id) {
                            visited.insert(n_id.clone());
                            if let Some(n_node) = self.nodes.get(n_id) {
                                let sim = cosine_similarity(query, &n_node.embedding);
                                candidates.push(SearchResult {
                                    id: n_id.clone(),
                                    similarity: sim,
                                });
                                queue.push(n_id.clone());
                            }
                        }
                    }
                }
            }
        }

        candidates.sort_by(|a, b| b.similarity.partial_cmp(&a.similarity).unwrap());
        if candidates.len() > ef {
            candidates.truncate(ef);
        }

        candidates
    }

    fn select_neighbors(&self, candidates: &[SearchResult], m: usize) -> Vec<String> {
        candidates.iter().take(m).map(|c| c.id.clone()).collect()
    }

    pub fn search(&self, query: &[f32], top_k: usize) -> Vec<SearchResult> {
        if self.entry_point.is_none() || self.nodes.is_empty() {
            return Vec::new();
        }

        let mut curr_obj = self.entry_point.clone().unwrap();
        let max_lvl = self.max_level;

        // Top layers search down to 1
        for level in (1..=max_lvl).rev() {
            curr_obj = self.search_layer_closest(query, &curr_obj, level);
        }

        // Layer 0 search
        let mut results = self.search_layer(query, &curr_obj, self.ef_search.max(top_k), 0);
        results.sort_by(|a, b| b.similarity.partial_cmp(&a.similarity).unwrap());
        if results.len() > top_k {
            results.truncate(top_k);
        }

        results
    }

    pub fn remove(&mut self, id: &str) -> bool {
        if let Some(node) = self.nodes.remove(id) {
            // Remove references from neighbor nodes
            for (level, n_list) in node.neighbors.iter().enumerate() {
                for n_id in n_list {
                    if let Some(neighbor_node) = self.nodes.get_mut(n_id) {
                        if level < neighbor_node.neighbors.len() {
                            neighbor_node.neighbors[level].retain(|x| x != id);
                        }
                    }
                }
            }

            if self.entry_point.as_deref() == Some(id) {
                self.entry_point = self.nodes.keys().next().cloned();
            }
            true
        } else {
            false
        }
    }

    pub fn clear(&mut self) {
        self.nodes.clear();
        self.entry_point = None;
        self.max_level = 0;
    }

    pub fn len(&self) -> usize {
        self.nodes.len()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_hnsw_insert_and_search() {
        let mut index = HnswIndex::default_hnsw();

        index.insert("v1".to_string(), vec![1.0, 0.0, 0.0]);
        index.insert("v2".to_string(), vec![0.0, 1.0, 0.0]);
        index.insert("v3".to_string(), vec![0.9, 0.1, 0.0]);
        index.insert("v4".to_string(), vec![0.0, 0.0, 1.0]);

        let query = vec![1.0, 0.0, 0.0];
        let results = index.search(&query, 2);

        assert_eq!(results.len(), 2);
        assert_eq!(results[0].id, "v1");
        assert_eq!(results[1].id, "v3");
    }

    #[test]
    fn test_hnsw_remove() {
        let mut index = HnswIndex::default_hnsw();
        index.insert("v1".to_string(), vec![1.0, 0.0, 0.0]);
        index.insert("v2".to_string(), vec![0.0, 1.0, 0.0]);

        assert_eq!(index.len(), 2);
        assert!(index.remove("v1"));
        assert_eq!(index.len(), 1);

        let results = index.search(&vec![1.0, 0.0, 0.0], 2);
        assert_eq!(results[0].id, "v2");
    }
}
