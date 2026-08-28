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

# Oracle XA Guide

Seata Go implements the Oracle XA resource through Oracle `DBMS_XA` PL/SQL calls.
It does not use MySQL-style `XA START` statements.

Oracle support is intentionally exposed through the vendor XA adapter registration
API. Applications provide their Oracle `database/sql/driver.Driver`, while Seata Go
provides the XA branch lifecycle implementation for `types.DBTypeOracle`.

```go
import (
	stdsql "database/sql"
	"database/sql/driver"

	seatasql "seata.apache.org/seata-go/v2/pkg/datasource/sql"
	"seata.apache.org/seata-go/v2/pkg/datasource/sql/types"
)

func registerOracleXA(oracleDriver driver.Driver) error {
	return seatasql.RegisterSeataXADriver("seata-xa-oracle", seatasql.SeataDriverDescriptor{
		DBType:     types.DBTypeOracle,
		Target:     oracleDriver,
		TargetName: "oracle",
		ParseDBName: func(dsn string) (string, error) {
			return parseOracleServiceName(dsn)
		},
	})
}

db, err := stdsql.Open("seata-xa-oracle", dsn)
```

`parseOracleServiceName` should return the Oracle service name, database name, or
schema name that your application uses to identify the resource. Do not log the
full DSN because Oracle connection strings commonly contain credentials.

## XA Mapping

| Seata Go method | Oracle call |
| --- | --- |
| `Start` | `DBMS_XA.XA_START` |
| `End` | `DBMS_XA.XA_END` |
| `XAPrepare` | `DBMS_XA.XA_PREPARE` |
| `Commit` | `DBMS_XA.XA_COMMIT` |
| `Rollback` | `DBMS_XA.XA_ROLLBACK` |
| `Recover` | `TABLE(DBMS_XA.XA_RECOVER())` |
| `Forget` | `DBMS_XA.XA_FORGET` |

Oracle XID values are mapped to `DBMS_XA_XID(formatid, gtrid, bqual)`:

- `formatid`: fixed to `9752`, matching Seata Java `XABranchXid`.
- `gtrid`: Seata global XID bytes.
- `bqual`: Seata branch qualifier bytes, encoded as `-<branchId>`.
- `gtrid` and `bqual` must each fit Oracle `RAW(64)`.

Seata Go rejects XID values that cannot be reversed from Oracle recovery output.
It does not silently hash or truncate oversized XIDs.

## Requirements

- Use an Oracle driver that can execute PL/SQL blocks through `ExecerContext` or
  `PrepareContext` plus `StmtExecContext`.
- Use an Oracle driver that can run recovery queries through `QueryerContext` or
  `PrepareContext` plus `StmtQueryContext`.
- Grant the application user permission to execute the `DBMS_XA` package.
- Confirm that the application can call `DBMS_XA.XA_RECOVER()` or has an
  equivalent recovery path before enabling production traffic.
- Keep prepared branches observable and clean them up after failed validation.

Example privilege statement:

```sql
GRANT EXECUTE ON DBMS_XA TO seata_user;
```

The exact grant command can vary by Oracle edition, container database layout,
and security policy. Verify it with your DBA.

## Local Tests

Oracle unit tests use mock `driver.Conn` implementations and do not require a
running Oracle database:

```bash
go test ./pkg/datasource/sql/xa -run Oracle -v
```

Full package tests:

```bash
go test ./pkg/datasource/sql/...
```

There is no default Oracle integration test in this repository yet. A real
Oracle validation must record:

- Oracle edition and version.
- Go driver name and version.
- Application user privileges.
- `DBMS_XA.XA_START`, `XA_END`, `XA_PREPARE`, `XA_COMMIT`, `XA_ROLLBACK`,
  `XA_RECOVER`, and `XA_FORGET` behavior.
- Returned errors for branch missing, duplicate second-phase callbacks,
  invalid XID, and insufficient privileges.
- Whether prepared branches survive connection close and can be completed from
  a recovery connection.

## Known Behavior

| Behavior | Notes |
| --- | --- |
| `TMNoFlags` on `Start` | Sent as Oracle loose branch flag to match Seata Java behavior. |
| `TMFail` on `End` | Mapped internally to `TMSuccess` before rollback because Oracle `DBMS_XA.XA_END` does not accept `TMFAIL` in this implementation path. |
| `XAPrepare` readonly | Treats both `DBMS_XA.XA_OK` and `DBMS_XA.XA_RDONLY` as successful prepare results. |
| Recovery scan | Uses `TABLE(DBMS_XA.XA_RECOVER())` and converts `formatid/gtrid/bqual` back to Seata branch XID strings. |
| Already-ended classification | Classifies `ORA-24756`, `ORA-24761`, `XAER_NOTA`, and wrapped `DBMS_XA` numeric `XAER_NOTA` code `-4` as already-ended conditions. |

## Troubleshooting

| Symptom | Check |
| --- | --- |
| `sql: driver does not support the use of Named Parameters` | The selected Oracle driver fell back to legacy statement execution and cannot bind the named parameters required by the DBMS_XA PL/SQL block. |
| Prepare or statement execution fails during DBMS_XA calls | Confirm that the selected Oracle driver supports PL/SQL blocks with named binds either through connection-level context methods or statement-level context methods. |
| `ORA-01031: insufficient privileges` | Grant access to `DBMS_XA` or run validation with a user that has the required package privileges. |
| `oracle xa gtrid exceeds RAW(64)` or `oracle xa bqual exceeds RAW(64)` | Shorten the global XID or use a future mapping strategy approved by the project. |
| Recovery returns an invalid branch qualifier | Confirm that all participating applications use the same Seata XID mapping and `formatid=9752`. |
