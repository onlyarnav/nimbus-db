use std::collections::HashMap;

/// Evaluates simple metadata filter expressions like `region = 'india'` or `region = 'india' AND category = 'invoice'`.
pub fn matches_filter(metadata: &HashMap<String, String>, filter_expr: &str) -> bool {
    let expr = filter_expr.trim();
    if expr.is_empty() {
        return true;
    }

    // Split expression by AND
    let clauses = expr.split(" AND ");
    for clause in clauses {
        let clause = clause.trim();
        if clause.is_empty() {
            continue;
        }

        if let Some((key, val)) = parse_equality_clause(clause) {
            match metadata.get(&key) {
                Some(actual_val) if actual_val == &val => continue,
                _ => return false,
            }
        } else {
            // Malformed clause returns false
            return false;
        }
    }

    true
}

fn parse_equality_clause(clause: &str) -> Option<(String, String)> {
    let parts: Vec<&str> = clause.split('=').collect();
    if parts.len() != 2 {
        return None;
    }

    let key = parts[0].trim().to_string();
    let mut val = parts[1].trim().to_string();

    // Strip outer single or double quotes if present
    if (val.starts_with('\'') && val.ends_with('\'')) || (val.starts_with('"') && val.ends_with('"')) {
        val = val[1..val.len() - 1].to_string();
    }

    Some((key, val))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_filter_matching() {
        let mut meta = HashMap::new();
        meta.insert("region".to_string(), "india".to_string());
        meta.insert("category".to_string(), "invoice".to_string());

        assert!(matches_filter(&meta, ""));
        assert!(matches_filter(&meta, "region = 'india'"));
        assert!(matches_filter(&meta, "region = \"india\" AND category = 'invoice'"));

        assert!(!matches_filter(&meta, "region = 'us-east'"));
        assert!(!matches_filter(&meta, "region = 'india' AND category = 'contract'"));
    }
}
