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

# MariaDB XA Guide

Seata Go provides a dedicated MariaDB XA driver name:

```go
db, err := sql.Open(
	"seata-xa-mariadb",
	"root:password@tcp(127.0.0.1:3306)/seata_demo?charset=utf8mb4&parseTime=True&multiStatements=true",
)
```

MariaDB support uses the existing MySQL wire protocol driver dependency and MariaDB's MySQL-compatible XA control statements:

- `XA START`
- `XA END`
- `XA PREPARE`
- `XA COMMIT`
- `XA ROLLBACK`
- `XA RECOVER`

## Requirements

- Use a MariaDB server version that supports XA transactions.
- Use transactional tables such as InnoDB.
- Grant the application user the privileges needed by its business SQL and XA control statements.
- Keep `multiStatements=true` if your application executes multiple statements through the same connection path.

## Known Behavior

MariaDB has its own `DBTypeMARIADB`, resource factory, and error classifier. It is not treated as a MySQL alias even though the first implementation uses MySQL-compatible XA SQL. This keeps MariaDB-specific version notes, recovery behavior, and error handling isolated from MySQL.

MariaDB XA connections are held for the second phase, matching the existing Seata Java behavior and the existing Go-side `DBResource.checkDbVersion()` policy.

## Local Integration Test

The MariaDB integration test is disabled by default. Set `SEATA_GO_TEST_MARIADB_DSN` and run it with the `integration` build tag:

```bash
SEATA_GO_TEST_MARIADB_DSN='root:password@tcp(127.0.0.1:3306)/seata_go_test?multiStatements=true&parseTime=true' \
  go test -tags integration ./pkg/datasource/sql/xa -run 'TestMariaDBXAConnIntegration' -v
```

The integration tests validate a prepared commit branch, a prepared rollback branch, `XA RECOVER`, cleanup of prepared branches, and duplicate second-phase callback behavior.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| `no XA resource factory registered for db type` | Confirm the application imports and initializes Seata Go SQL support and uses `seata-xa-mariadb`. |
| `invalid arguments` from XA resource methods | Check the XA flag passed by the caller. `Start` accepts `TMNoFlags`, `TMJoin`, and `TMResume`; `End` accepts `TMSuccess`, `TMFail`, and `TMSuspend`. |
| Prepared branches remain after failure | Run `XA RECOVER` on MariaDB and complete the branch with `XA COMMIT '<xid>'` or `XA ROLLBACK '<xid>'` after confirming the global transaction decision. |
| XA statements fail on non-transactional tables | Use transactional storage engines such as InnoDB. |
