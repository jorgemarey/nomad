package state

import memdb "github.com/hashicorp/go-memdb"

const (
	TableSentinelPolicies = "sentinel_policy"
)

func init() {
	// Register premium schemas
	RegisterSchemaFactories([]SchemaFactory{
		sentinelPolicyTableSchema,
	}...)
}

// sentinelPolicyTableSchema returns the MemDB schema for the sentinel policy
// table. This table is used to store the policies which are enforced.
func sentinelPolicyTableSchema() *memdb.TableSchema {
	return &memdb.TableSchema{
		Name: TableSentinelPolicies,
		Indexes: map[string]*memdb.IndexSchema{
			"id": {
				Name:         "id",
				AllowMissing: false,
				Unique:       true,
				Indexer: &memdb.StringFieldIndex{
					Field: "Name",
				},
			},
			"scope": {
				Name:         "scope",
				AllowMissing: false,
				Unique:       false,
				Indexer: &memdb.StringFieldIndex{
					Field: "Scope",
				},
			},
		},
	}
}
