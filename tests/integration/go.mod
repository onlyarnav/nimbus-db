module github.com/onlyarnav/nimbusdb/tests/integration

go 1.25.0

require (
	github.com/onlyarnav/nimbusdb/services/control-plane v0.0.0
	github.com/onlyarnav/nimbusdb/services/gateway v0.0.0
	github.com/onlyarnav/nimbusdb/services/metadata-service v0.0.0
	google.golang.org/grpc v1.82.0
	google.golang.org/protobuf v1.36.11
)

replace github.com/onlyarnav/nimbusdb/services/metadata-service => ../../services/metadata-service
replace github.com/onlyarnav/nimbusdb/services/gateway => ../../services/gateway
replace github.com/onlyarnav/nimbusdb/services/control-plane => ../../services/control-plane


require (
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260414002931-afd174a4e478 // indirect
)
