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

# XA Multi-Database Delivery Notes

This document summarizes the local delivery scope for XA multi-database support and the checks to complete before proposing Apache pull requests.

Use [`xa_multi_db_pr_readiness.md`](./xa_multi_db_pr_readiness.md) as the
pre-PR requirement-to-evidence checklist.

## Implemented Scope

| Area | Status |
| --- | --- |
| MySQL recover hygiene | The existing MySQL XA resource now closes `XA RECOVER` rows and accepts both string and `[]byte` recover payloads, matching the recover data shapes already covered for MariaDB. |
| MySQL/MariaDB XID quoting | MySQL-compatible XA control statements now quote XA XIDs through a shared helper, so embedded single quotes are escaped before `XA START`, `XA END`, `XA PREPARE`, `XA COMMIT`, or `XA ROLLBACK` is executed. |
| PostgreSQL | Keeps the prepared-transaction XA resource, quotes prepared-transaction XIDs in SQL literals, closes `pg_prepared_xacts` recovery rows, accepts recover payloads as string or `[]byte`, and classifies `pgconn.PgError` or text errors with SQLSTATE `42704` / `55000` as already-ended for phase-two idempotency handling. |
| MariaDB | Adds `seata-xa-mariadb`, a MariaDB XA resource factory, MySQL-compatible XA lifecycle statements, recovery parsing, MariaDB-specific error classification, XAConn autoCommit plus explicit commit/rollback, timeout/prepare-failure, TC report-failure, phase-two held-connection release, and phase-two failure-status coverage, integration tests, and user documentation. |
| SQL XA driver fallback | MySQL, MariaDB, and PostgreSQL XA control and recovery statements now use the shared driver prepare fallback so drivers that return `driver.ErrSkip` from connection-level exec/query can still run the statement on the same physical connection. |
| Oracle | Adds Oracle `DBMS_XA` XID mapping, lifecycle calls, readonly prepare status reporting, recovery parsing with row cleanup, prepared-statement fallback, already-ended error classification, DBMS_XA driver transaction state management, XAConn autoCommit plus explicit commit/rollback, timeout/prepare-failure, TC report-failure, phase-two held-connection release, phase-two failure-status coverage, unit tests, setup/troubleshooting documentation, and Oracle Free plus go-ora external validation. |
| XA prepared statements | `XAConn.PrepareContext` now returns statements whose `StmtExecContext` / `StmtQueryContext` executions enter the same XA branch lifecycle as direct `ExecContext` / `QueryContext`; query rows still defer branch commit until `Rows.Close`. |
| XA resource contract tests | Adds shared contract coverage for supported XA resource factories, resource/classifier registration, unknown DB type rejection, common recovery-scan flag behavior, transaction-timeout defaults, `IsSameRM`, and `XAResourcePrepareStatus` capability. |
| Datasource resource group | `DBResource.GetResourceGroupId` now returns the current RM transaction service group instead of panicking, keeping the datasource resource interface aligned with the RM registration contract. |
| Vendor adapters | Adds `RegisterSeataXADriver` and `SeataDriverDescriptor` so applications can register vendor `database/sql/driver.Driver` implementations without adding them as Seata Go dependencies. |
| Dameng prototype | Adds `types.DBTypeDM` and a `DBMS_XA`-based XA resource prototype with XID mapping, lifecycle calls, readonly prepare status reporting, recovery parsing with row cleanup, error classification, XAConn autoCommit plus explicit commit/rollback, timeout/prepare-failure, TC report-failure, phase-two held-connection release, and phase-two failure-status coverage, unit tests, and documentation. |
| Kingbase and Oscar | Documents extension direction and open questions. Kingbase should be validated first against PostgreSQL-style prepared transactions. Oscar needs public Go driver, XA API, recovery, and error-code confirmation before code is added. |

## Verification Commands

Run the following checks before sending or updating the pull requests:

```bash
git diff --check
go test ./pkg/datasource/sql/types -run 'DBType|ParseDBType|IndexConstants' -v
go test ./pkg/protocol/branch ./pkg/protocol/codec -run 'TestBranchStatus|TestBranchReportRequestCodec' -v
go test ./pkg/rm/remoting/grpc -run 'TestGetGrpcRMRemotingInstance|TestGrpcRMRemotingBranchRegisterXAType|TestGrpcRMRemotingBranchReportReadonlyStatus' -v
go test ./pkg/remoting/grpc ./pkg/remoting/processor/client
go test ./pkg/datasource/sql/xa -run 'TestXAResource' -v
go test ./pkg/datasource/sql/xa -run 'Postgres|MariaDB|Oracle|DM' -v
go test ./pkg/datasource/sql/xa -run 'TestPostgresXAConn_Recover|TestPostgresXAErrorClassifierIsAlreadyEnded' -v
go test ./pkg/datasource/sql/xa -run 'TestOracleXAConnRecover|TestOracleXARecoverFallsBackToPrepare|TestDMXAConnRecover|TestDMXARecoverFallsBackToPrepare' -v
go test ./pkg/datasource/sql/xa -run 'TestMysqlXAConn_Recover|TestMariaDBXAConnRecover' -v
go test ./pkg/datasource/sql/xa -run 'TestMysqlXAConn_LifecycleSQLQuotesXID|TestMariaDBXAConnLifecycleSQL' -v
go test ./pkg/datasource/sql/xa -run 'TestPostgresXAConnLifecycleSQLQuotesXID' -v
go test ./pkg/datasource/sql/xa -run 'Test(Mysql|MariaDB|Postgres)XAConn.*FallsBackToPrepare' -v
go test ./pkg/datasource/sql -run 'TestXAConn_PreparedExecContext_AutoCommitCompletesXABranch|TestXAConn_PreparedQueryContext_AutoCommitDefersBranchCommitUntilRowsClose' -v
go test ./pkg/datasource/sql -run 'TestRegisterSeataXADriver|TestXAConn_BeginTx_DBMSXADriverTxHeldUntilPhaseTwo|TestXAConn_BeginTx_DBMSXAReadonlyPrepareReportsReadonlyAndReleases' -v
go test ./pkg/datasource/sql -run 'TestDBResourceGetResourceGroupIdUsesRMConfig|TestDBResourceCheckDbVersionControlsXAConnectionHold|TestXAResourceManager|TestXAConn_BeginTx|TestXAConn_ExecContext|TestXAConn_AutoCommit|TestXATx' -v
go test ./pkg/datasource/sql/xa ./pkg/datasource/sql/types
go test ./pkg/datasource/sql/...
go test ./...
```

Run the MariaDB integration test when a MariaDB DSN is available:

```bash
SEATA_GO_TEST_MARIADB_DSN='user:password@tcp(127.0.0.1:3306)/seata_demo?parseTime=true' \
  go test -count=1 -tags integration ./pkg/datasource/sql/xa -run 'TestMariaDBXAConnIntegration' -v
```

Local Docker validation for this delivery branch passed against `mariadb:11.4`
(`11.4.13-MariaDB-ubu2404`) for commit, rollback, recover cleanup, and duplicate
second-phase callback coverage. Duplicate callbacks returned MariaDB
`Error 1397: XAER_NOTA: Unknown XID` and did not leave the completed XIDs in
`XA RECOVER`.

External Oracle validation for this delivery branch passed against
`gvenzl/oracle-free:23-slim-faststart` with Oracle Free `23.26.2.0.0` and
`github.com/sijms/go-ora/v2 v2.9.0`. The probe registered a
`seata-xa-oracle`-style wrapper through `RegisterSeataXADriver`, completed
`XAConn.BeginTx` / `XATx.Commit` phase-one prepare without a manual application
`BeginTx`, and completed phase-two commit through the held XA connection. The
same environment confirmed `DBMS_XA.XA_START` with `TMNOFLAGS`, rollback branch,
recover visibility and cleanup, and readonly prepare returning `XA_RDONLY`.
The reproducible external-module flow is documented in
[`xa_oracle_go_ora_validation.md`](./xa_oracle_go_ora_validation.md) without
adding go-ora to the project dependency graph.

## Compliance Check

| Check | Current Result |
| --- | --- |
| New dependencies | No `go.mod` or `go.sum` changes are required by the current implementation. |
| Vendor drivers | Oracle and Dameng drivers are injected by applications through the vendor adapter API; they are not added as direct project dependencies. |
| Recover row cleanup | MySQL, MariaDB, PostgreSQL, Oracle, and Dameng recover readers close returned rows; SQL recover paths accept driver data as string or bytes, and DBMS_XA recover paths accept numeric/string/bytes row fields covered by unit tests. |
| XID SQL literal quoting | MySQL, MariaDB, and PostgreSQL XA control statements escape embedded single quotes in branch XIDs before building SQL string literals. |
| Driver prepare fallback | MySQL, MariaDB, PostgreSQL, Oracle, and Dameng XA exec/query paths use the shared prepare fallback where the underlying driver can legally return `driver.ErrSkip`; fallback tests cover both SQL XA and DBMS_XA resources. |
| DBMS_XA driver transaction state | Oracle and Dameng XA branches open a driver-level transaction before `XA_START` to keep DBMS_XA-capable drivers out of autoCommit mode; normal prepared branches keep it until phase two, while rollback, readonly, failure, close, and force-close paths clean it up. |
| Oracle/DM version probing | Oracle and Dameng resource initialization skips MySQL-style `SELECT VERSION()` probing because their hold policy does not depend on parsed server version and some drivers reject that query. |
| XA connection hold policy | `DBResource.checkDbVersion` owns the hold decision: MySQL versions before 8.0.29, MariaDB, Oracle, and Dameng hold prepared connections; MySQL 8.0.29+ and PostgreSQL do not get held only because their DB type is known. |
| Prepared statement XA lifecycle | Context-aware prepared statement executions join the XA branch lifecycle at execution time, not prepare time, and prepared query rows keep the existing close-time branch commit behavior. |
| XA resource contract | MySQL, PostgreSQL, MariaDB, Oracle, and Dameng are covered by a shared factory/resource/classifier contract test; unsupported DB types keep the default no-op classifier and return a clear resource factory error. |
| Datasource resource group | `DBResource.GetResourceGroupId` follows `rm.GetRmConfig().TxServiceGroup`, so the required `rm.Resource` method is safe to call and remains consistent with the transaction service group used by RM registration requests. |
| PostgreSQL phase-two idempotency | PostgreSQL SQLSTATE `42704` and `55000` are classified as already-ended errors, so duplicate or late phase-two failures can cache committed/rollbacked status consistently with the RM contract. |
| Readonly prepare status | The protocol enum now includes `BranchStatusPhaseoneReadonly = 13`, matching the existing gRPC `PhaseOne_RDONLY` value; normal codec and gRPC branch-report request coverage both preserve the readonly status. |
| XA branch type over gRPC | `BranchTypeProto` now explicitly includes `XA = 3`, matching the normal protocol `BranchTypeXA` value used by XA branch register, report, commit, and rollback messages; gRPC branch-register, branch-report, and branch-end processor tests cover the XA mapping. |
| License headers | New Go and Markdown files include the Apache Software Foundation license header. |
| Generated files | `dbtype_string.go` is updated together with DB type tests. |
| Secrets | Documentation examples use placeholders and must not include real DSNs, passwords, wallets, or private deployment details. |

## Suggested Pull Request Split

| PR | Scope | Notes |
| --- | --- | --- |
| PR 1 | Java/Go comparison, XA contract notes, and MariaDB resource | Keeps the first database support reviewable and includes the MariaDB integration path. |
| PR 2 | Oracle XID helper and `DBMS_XA` lifecycle | Focuses review on XID encoding, PL/SQL block construction, and lifecycle mapping. |
| PR 3 | Oracle recovery, classifier, fallback, and documentation | Groups recovery semantics, driver fallback, and user-facing Oracle setup. |
| PR 4 | Vendor XA adapter registration | Reviews public API shape for external driver injection. |
| PR 5 | Dameng prototype and domestic database extension notes | Presents Dameng as a prototype until real-driver validation is available. |

The split can be squashed differently if maintainers prefer fewer pull requests, but the review description should keep these boundaries explicit.

## Remaining Validation

| Priority | Item | Why It Matters |
| --- | --- | --- |
| P1 | Broader Oracle validation matrix | Oracle Free plus go-ora now confirms the wrapper path, named binds, output binds, privileges, recovery, readonly prepare, and held-connection phase-two commit. Additional Oracle editions, drivers, duplicate callbacks, branch-missing errors, and connection-loss cases should still be recorded before broad production wording. |
| P0 | Dameng driver and license validation | The prototype intentionally avoids a direct driver dependency until the official driver source, versioning, license, and redistribution terms are clear. The required checks are listed in [`xa_dm_validation.md`](./xa_dm_validation.md). |
| P1 | Dameng real database validation | `DBMS_XA` availability, `XA_COMPATIBLE_MODE`, recovery shape, and real error codes must be verified before marking Dameng production-ready. The lifecycle and error checklist is listed in [`xa_dm_validation.md`](./xa_dm_validation.md). |
| P2 | Kingbase prototype decision | A PostgreSQL prepared-transaction-based prototype is plausible, but it should wait for driver and compatibility confirmation. |
| P2 | Oscar follow-up | Add code only after public Go driver, XA API, recovery, and error-code evidence is available. |

## Support-Level Wording

Use precise wording in release notes and pull request descriptions:

- MariaDB: supported XA resource with unit tests, documentation, and MariaDB
  11.4.13 integration coverage for lifecycle, recovery cleanup, and duplicate
  second-phase callbacks.
- PostgreSQL: existing prepared-transaction XA resource with XID SQL literal quoting, recover row cleanup, and SQLSTATE-based already-ended classification for phase-two idempotency.
- Oracle: `DBMS_XA` implementation with mock coverage, setup documentation, and Oracle Free plus go-ora external validation for wrapper phase-one prepare, phase-two commit, rollback, recover, and readonly prepare; broader driver/edition validation still recommended before production-ready wording.
- Vendor adapter: public extension API for wrapping external drivers without adding direct dependencies.
- Dameng: prototype resource with mock coverage for readonly prepare status propagation; keep it prototype-only until driver licensing, compatibility mode, recovery output, and error codes are validated on a real database.
