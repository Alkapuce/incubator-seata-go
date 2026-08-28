<!--
 Licensed to the Apache Software Foundation (ASF) under one or more
 contributor license agreements.  See the NOTICE file distributed with
 this work for additional information regarding copyright ownership.
 The ASF licenses this file to You under the Apache License, Version 2.0
 (the "License"); you may not use this file except in compliance with
 the License.  You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
-->

# Vendor XA Adapter Guide

Seata Go can wrap an external `database/sql/driver.Driver` as a Seata XA driver without adding the vendor driver as a direct project dependency.

Register the vendor driver during application initialization, before `sql.Open` uses the new driver name:

```go
import (
	stdsql "database/sql"

	seatasql "seata.apache.org/seata-go/v2/pkg/datasource/sql"
	"seata.apache.org/seata-go/v2/pkg/datasource/sql/datasource"
	"seata.apache.org/seata-go/v2/pkg/datasource/sql/types"
)

err := seatasql.RegisterSeataXADriver("seata-xa-vendor", seatasql.SeataDriverDescriptor{
	DBType:     types.DBTypeOracle,
	Target:     vendorDriver,
	TargetName: "vendor-driver-name",
	ParseDBName: func(dsn string) (string, error) {
		return parseVendorDBName(dsn)
	},
	NewTableMetaCache: func(db *stdsql.DB, dbName string) datasource.TableMetaCache {
		return vendorTableMetaCache(db, dbName)
	},
})
```

Then open the database through the registered Seata XA driver name:

```go
db, err := stdsql.Open("seata-xa-vendor", dsn)
```

## Descriptor Fields

| Field | Required | Purpose |
| --- | --- | --- |
| `DBType` | Yes | Selects the XA resource factory and database-specific behavior. |
| `Target` | Yes | The underlying vendor `driver.Driver`. |
| `TargetName` | No | Name reported by the wrapped connector. Defaults to `DBType.String()`. |
| `ParseDBName` | Yes | Extracts the database/schema name from the vendor DSN. |
| `NewTableMetaCache` | No | Registers table metadata support when the vendor adapter needs it. Pure XA adapters can leave it nil. |

## Adapter Checklist

- Register or implement an XA resource factory for the vendor `DBType`.
- Provide a stable Seata driver name, for example `seata-xa-vendor`.
- Parse the database name from the vendor DSN without logging credentials.
- Add unit tests that prove the wrapper returns an `XAConn` with the expected DB type and database name.
- Add integration tests for start, end, prepare, commit, rollback, recover, branch-missing behavior, and repeated second-phase callbacks.
- Document driver version, database version, permissions, recovery behavior, and unsupported XA flags.
