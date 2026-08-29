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

# Dameng XA Prototype Guide

Seata Go provides a Dameng XA resource prototype through the `DBMS_XA` PL/SQL
package and `types.DBTypeDM`. The prototype does not add a Dameng driver
dependency to Seata Go. Applications must register their selected Dameng
`database/sql/driver.Driver` through the vendor adapter API before opening the
database.

```go
import (
	stdsql "database/sql"
	"database/sql/driver"

	seatasql "seata.apache.org/seata-go/v2/pkg/datasource/sql"
	"seata.apache.org/seata-go/v2/pkg/datasource/sql/types"
)

func registerDMXA(dmDriver driver.Driver) error {
	return seatasql.RegisterSeataXADriver("seata-xa-dm", seatasql.SeataDriverDescriptor{
		DBType:     types.DBTypeDM,
		Target:     dmDriver,
		TargetName: "dm",
		ParseDBName: func(dsn string) (string, error) {
			return parseDMDBName(dsn)
		},
	})
}

db, err := stdsql.Open("seata-xa-dm", dsn)
```

`parseDMDBName` should return the Dameng schema, database, or service identifier
used by the application as the Seata resource name. Do not log the full DSN.

## XA Mapping

| Seata Go method | Dameng call |
| --- | --- |
| `Start` | `DBMS_XA.XA_START` |
| `End` | `DBMS_XA.XA_END` |
| `XAPrepare` | `DBMS_XA.XA_PREPARE` |
| `Commit` | `DBMS_XA.XA_COMMIT` |
| `Rollback` | `DBMS_XA.XA_ROLLBACK` |
| `Recover` | `TABLE(DBMS_XA.XA_RECOVER())` |
| `Forget` | `DBMS_XA.XA_FORGET` |

The prototype maps Seata branch XIDs to `DBMS_XA_XID(formatid, gtrid, bqual)`:

- `formatid`: fixed to `9752`, matching Seata Java `XABranchXid`.
- `gtrid`: Seata global XID bytes.
- `bqual`: Seata branch qualifier bytes, encoded as `-<branchId>`.
- `gtrid` and `bqual` must each fit `RAW(64)`.

## Requirements

- Use a Dameng version and compatibility mode that supports `DBMS_XA`.
- Use a Dameng Go driver that can execute PL/SQL blocks through `ExecerContext`
  or `PrepareContext` plus `StmtExecContext`.
- Use a Dameng Go driver that supports `database/sql.Out` named output binds if
  the application needs `XA_RDONLY` prepare branches to be reported as
  `BranchStatusPhaseoneReadonly`.
- Use a Dameng Go driver that can run recovery queries through `QueryerContext`
  or `PrepareContext` plus `StmtQueryContext`.
- Confirm the driver source, license, and redistribution terms before adding it
  to any Apache repository dependency.
- Validate `XA_RECOVER`, branch-missing behavior, duplicate second-phase
  callbacks, and invalid XID errors against a real Dameng database before
  describing the prototype as supported or enabling production traffic.

See [Dameng XA Validation Checklist](./xa_dm_validation.md) for the required
real-driver and real-database validation items.

## Local Tests

The Dameng prototype tests use mock `driver.Conn` implementations and do not
require a running Dameng database:

```bash
go test ./pkg/datasource/sql/xa -run DM -v
go test ./pkg/datasource/sql/types -run 'DBType|ParseDBType' -v
```

Full SQL package tests:

```bash
go test ./pkg/datasource/sql/...
```

## Known Limitations

| Limitation | Status |
| --- | --- |
| Real database validation | Not completed in this repository yet. |
| Driver dependency | Not added to `go.mod`; applications register the driver externally. |
| Support level | Prototype only until the real-driver and real-database checklist is complete. |
| Compatibility mode | `DBMS_XA` availability must be verified against the target Dameng deployment; Dameng documents that MySQL-compatible XA mode does not use the `DBMS_XA` package. |
| PL/SQL and recover shape | The prototype uses Oracle-like anonymous blocks and a `TABLE(DBMS_XA.XA_RECOVER())` query shape; both must be checked against the selected Dameng version and driver. |
| Readonly prepare reporting | The Go side reads `DBMS_XA.XA_PREPARE` through an output bind and reports `BranchStatusPhaseoneReadonly` when `XA_RDONLY` is returned; this bind behavior still needs real-driver validation. |
| Error semantics | Only generic `XAER_NOTA` text and numeric `-4` are classified as already-ended until real errors are captured. |
| Table metadata | No Dameng table metadata cache is implemented by this prototype. |
