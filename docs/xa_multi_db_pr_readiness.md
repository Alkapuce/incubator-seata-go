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

# XA Multi-Database PR Readiness Checklist

This checklist maps the original XA multi-database requirements to the local
implementation, documentation, and validation evidence that should be referenced
before opening or updating Apache pull requests.

## Requirement-to-Evidence Matrix

| Requirement | Current Evidence | PR Wording |
| --- | --- | --- |
| Compare Seata Java and Seata Go XA database support, control statements, recovery, and error semantics. | `docs/xa_multi_db_delivery.md` and `docs/xa_multi_db_delivery_zh.md` summarize the Go-side delivery. The external project note `docs/seata-go-xa-java-go-db-support-comparison.md` keeps the Java/Go/database comparison for review preparation. | Completed as local review material. Move or trim the comparison if maintainers prefer a shorter in-repository PR note. |
| Add MariaDB XA support. | `pkg/datasource/sql/xa/mariadb_xa_connection.go`, `pkg/datasource/sql/driver.go`, `pkg/datasource/sql/types/types.go`, `docs/xa_mariadb.md`, `docs/xa_mariadb_zh.md`, and `docs/quickstart*.md` cover the driver name, DB type, lifecycle, recovery, classifier, usage, and troubleshooting path. Local Docker validation passed against MariaDB `11.4.13-MariaDB-ubu2404` for commit, rollback, recovery cleanup, and duplicate phase-two callbacks. | Supported XA resource with unit tests, documentation, and MariaDB 11.4.13 integration coverage. |
| Add Oracle XA support aligned with Seata Java's `DBMS_XA` path where practical. | `pkg/datasource/sql/xa/oracle_xa_connection.go`, `oracle_xa_plsql.go`, `oracle_xid.go`, `docs/xa_oracle.md`, `docs/xa_oracle_zh.md`, and `docs/xa_oracle_go_ora_validation*.md` cover XID mapping, `DBMS_XA` lifecycle calls, recovery, readonly prepare, classifier behavior, driver prepare fallback, and external driver validation. Oracle Free `23.26.2.0.0` plus `github.com/sijms/go-ora/v2 v2.9.0` validated the wrapper path and direct resource checks. | `DBMS_XA` implementation with mock coverage, setup docs, and Oracle Free plus go-ora external validation. Broader driver and edition validation is still recommended before production-ready wording. |
| Provide a reusable vendor adapter and domestic database extension path, with at least one prototype. | `RegisterSeataXADriver` and `SeataDriverDescriptor` in `pkg/datasource/sql/driver.go` expose the driver-injection API. `pkg/datasource/sql/xa/dm_xa_connection.go`, `dm_xa_plsql.go`, `dm_xid.go`, `docs/xa_dm*.md`, and `docs/xa_dm_validation*.md` provide the Dameng prototype and the validation checklist. Kingbase and Oscar remain documented follow-ups in the delivery notes. | Vendor adapter is ready for review. Dameng must stay prototype-only until real driver, license, compatibility mode, recovery output, and error-code validation are complete. |
| Document configuration, permissions, examples, troubleshooting, validation, and Apache compliance boundaries. | `docs/quickstart*.md`, `docs/xa_mariadb*.md`, `docs/xa_oracle*.md`, `docs/xa_oracle_go_ora_validation*.md`, `docs/xa_vendor_adapter*.md`, `docs/xa_dm*.md`, and `docs/xa_dm_validation*.md` cover setup and support-level language. Current implementation adds no direct Oracle or Dameng driver dependency, and new Go/Markdown files carry ASF license headers. | Ready as a review package. Keep secrets out of examples and keep external driver dependencies outside this repository unless license and redistribution terms are confirmed. |

## Commands to Rerun Before PR

Run these checks from the repository root:

```bash
git diff --check
go test ./pkg/datasource/sql/types -run 'DBType|ParseDBType|IndexConstants' -v
go test ./pkg/protocol/branch ./pkg/protocol/codec -run 'TestBranchStatus|TestBranchReportRequestCodec' -v
go test ./pkg/rm/remoting/grpc -run 'TestGetGrpcRMRemotingInstance|TestGrpcRMRemotingBranchRegisterXAType|TestGrpcRMRemotingBranchReportReadonlyStatus' -v
go test ./pkg/remoting/grpc ./pkg/remoting/processor/client
go test ./pkg/datasource/sql/xa -run 'Postgres|MariaDB|Oracle|DM' -v
go test ./pkg/datasource/sql -run 'TestRegisterSeataXADriver|TestXAConn_BeginTx_DBMSXADriverTxHeldUntilPhaseTwo|TestXAConn_BeginTx_DBMSXAReadonlyPrepareReportsReadonlyAndReleases' -v
go test ./pkg/datasource/sql/xa ./pkg/datasource/sql/types
go test ./pkg/datasource/sql/...
go test ./...
```

Run the MariaDB integration test when a local DSN is available:

```bash
SEATA_GO_TEST_MARIADB_DSN='user:password@tcp(127.0.0.1:3306)/seata_demo?parseTime=true' \
  go test -count=1 -tags integration ./pkg/datasource/sql/xa -run 'TestMariaDBXAConnIntegration' -v
```

Run the license-header scan against the proposed PR range:

```bash
python3 - <<'PY'
import subprocess

files = subprocess.check_output(
    [
        "git",
        "diff",
        "--name-only",
        "--diff-filter=ACMRT",
        "origin/master..HEAD",
        "--",
        "*.go",
        "*.md",
    ],
    text=True,
).splitlines()
missing = []
for path in files:
    with open(path, "r", encoding="utf-8", errors="replace") as fh:
        head = "".join(fh.readline() for _ in range(25))
    if "Licensed to the Apache Software Foundation (ASF)" not in head:
        missing.append(path)

print(f"checked_go_md_files={len(files)}")
print(f"missing_license_header={len(missing)}")
for path in missing:
    print(path)
PY
```

## External Validation Records

Record the following facts in the PR description or linked validation notes:

- MariaDB image and server version, DSN shape with credentials removed, test
  command, and duplicate callback behavior.
- Oracle image, edition, version, PDB or service name, external Go driver name
  and version, granted privileges, wrapper probe result, direct resource checks,
  and any observed duplicate callback or connection-loss errors.
- Dameng driver source, version, license, redistribution terms, `database/sql`
  driver name, Go version, `XA_COMPATIBLE_MODE`, `DBMS_XA` availability,
  recovery result shape, and real error text before changing support wording.

## Blocking and Non-Blocking Items

| Item | PR Impact |
| --- | --- |
| MariaDB lifecycle, recovery cleanup, and duplicate phase-two callback validation | Not blocking. This is already covered by local Docker integration evidence. |
| Oracle Free plus go-ora wrapper and direct `DBMS_XA` validation | Not blocking for an Oracle implementation PR when the PR wording names the tested driver and edition. Broader Oracle driver and edition validation remains a follow-up. |
| Dameng real database and driver validation | Blocking for supported Dameng wording. Not blocking for a prototype PR if the code and docs keep the current prototype language. |
| Kingbase and Oscar confirmation | Not blocking for the current prototype scope. Keep them as documented follow-ups until driver, XA API, recovery, and error-code evidence is available. |

## Do Not Include

- Real DSNs, passwords, wallet paths, private hostnames, or internal deployment
  details in committed files, logs, screenshots, or PR text.
- Direct Oracle or Dameng driver dependencies unless the source, license, and
  redistribution terms are confirmed and accepted by maintainers.
- Production-ready Dameng, Kingbase, or Oscar wording without real database,
  driver, recovery, and error-semantics evidence.
- Generated or environment-specific artifacts such as temporary probe modules,
  database dumps, build directories, and raw container logs.
