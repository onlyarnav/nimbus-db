use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use tonic::{transport::Server, Request, Response, Status};

pub mod node_agent_proto {
    tonic::include_proto!("nodeagent");
}

use node_agent_proto::node_agent_server::{NodeAgent, NodeAgentServer};
use node_agent_proto::*;
use node_agent::storage::engine::StorageEngine;
use node_agent::storage::replication::ReplicationRole;

#[derive(Default)]
pub struct NodeAgentService {
    engines: Arc<Mutex<HashMap<String, StorageEngine>>>,
    data_root: String,
}

impl NodeAgentService {
    pub fn new(data_root: &str) -> Self {
        NodeAgentService {
            engines: Arc::new(Mutex::new(HashMap::new())),
            data_root: data_root.to_string(),
        }
    }
}

#[tonic::async_trait]
impl NodeAgent for NodeAgentService {
    async fn create_database(
        &self,
        request: Request<CreateDatabaseRequest>,
    ) -> Result<Response<CreateDatabaseResponse>, Status> {
        let req = request.into_inner();
        let db_id = req.database_id;
        let db_dir = format!("{}/{}", self.data_root, db_id);

        let mut engines = self.engines.lock().map_err(|e| Status::internal(e.to_string()))?;
        if engines.contains_key(&db_id) {
            return Ok(Response::new(CreateDatabaseResponse {
                success: false,
                endpoint: "".to_string(),
                error: format!("Database {} already exists on node", db_id),
            }));
        }

        match StorageEngine::open(&db_id, &db_dir, ReplicationRole::Leader) {
            Ok(engine) => {
                engines.insert(db_id.clone(), engine);
                Ok(Response::new(CreateDatabaseResponse {
                    success: true,
                    endpoint: format!("storage://localhost:50051/{}", db_id),
                    error: "".to_string(),
                }))
            }
            Err(e) => Ok(Response::new(CreateDatabaseResponse {
                success: false,
                endpoint: "".to_string(),
                error: format!("Failed to initialize storage engine: {}", e),
            })),
        }
    }

    async fn delete_database(
        &self,
        request: Request<DeleteDatabaseRequest>,
    ) -> Result<Response<DeleteDatabaseResponse>, Status> {
        let req = request.into_inner();
        let db_id = req.database_id;

        let mut engines = self.engines.lock().map_err(|e| Status::internal(e.to_string()))?;
        engines.remove(&db_id);

        let db_dir = format!("{}/{}", self.data_root, db_id);
        let _ = std::fs::remove_dir_all(db_dir);

        Ok(Response::new(DeleteDatabaseResponse {
            success: true,
            error: "".to_string(),
        }))
    }

    async fn backup_database(
        &self,
        request: Request<BackupDatabaseRequest>,
    ) -> Result<Response<BackupDatabaseResponse>, Status> {
        let req = request.into_inner();
        let db_id = req.database_id;

        let mut engines = self.engines.lock().map_err(|e| Status::internal(e.to_string()))?;
        if let Some(engine) = engines.get_mut(&db_id) {
            match engine.backup() {
                Ok(path) => Ok(Response::new(BackupDatabaseResponse {
                    success: true,
                    backup_path: path,
                    error: "".to_string(),
                })),
                Err(e) => Ok(Response::new(BackupDatabaseResponse {
                    success: false,
                    backup_path: "".to_string(),
                    error: format!("Backup failed: {}", e),
                })),
            }
        } else {
            Ok(Response::new(BackupDatabaseResponse {
                success: false,
                backup_path: "".to_string(),
                error: format!("Database {} not found", db_id),
            }))
        }
    }

    async fn restore_database(
        &self,
        request: Request<RestoreDatabaseRequest>,
    ) -> Result<Response<RestoreDatabaseResponse>, Status> {
        let req = request.into_inner();
        let db_id = req.database_id;
        let backup_path = req.backup_path;

        let mut engines = self.engines.lock().map_err(|e| Status::internal(e.to_string()))?;
        if let Some(engine) = engines.get_mut(&db_id) {
            match engine.restore(&backup_path) {
                Ok(_) => Ok(Response::new(RestoreDatabaseResponse {
                    success: true,
                    error: "".to_string(),
                })),
                Err(e) => Ok(Response::new(RestoreDatabaseResponse {
                    success: false,
                    error: format!("Restore failed: {}", e),
                })),
            }
        } else {
            let db_dir = format!("{}/{}", self.data_root, db_id);
            match StorageEngine::open(&db_id, &db_dir, ReplicationRole::Leader) {
                Ok(mut engine) => {
                    match engine.restore(&backup_path) {
                        Ok(_) => {
                            engines.insert(db_id, engine);
                            Ok(Response::new(RestoreDatabaseResponse {
                                success: true,
                                error: "".to_string(),
                            }))
                        }
                        Err(e) => Ok(Response::new(RestoreDatabaseResponse {
                            success: false,
                            error: format!("Restore failed: {}", e),
                        })),
                    }
                }
                Err(e) => Ok(Response::new(RestoreDatabaseResponse {
                    success: false,
                    error: format!("Failed to open engine for restore: {}", e),
                })),
            }
        }
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let addr = "0.0.0.0:50051".parse()?;
    let service = NodeAgentService::new("data");

    println!("NodeAgent Rust storage server listening on {}", addr);

    Server::builder()
        .add_service(NodeAgentServer::new(service))
        .serve(addr)
        .await?;

    Ok(())
}
