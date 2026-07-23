module github.com/onlyarnav/nimbusdb/services/gateway

go 1.25.0

require (
	github.com/onlyarnav/nimbusdb/services/metadata-service v0.0.0
	google.golang.org/grpc v1.82.0
	google.golang.org/protobuf v1.36.11
)

replace github.com/onlyarnav/nimbusdb/services/metadata-service => ../metadata-service
