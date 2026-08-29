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

# Oracle go-ora Validation Notes

This document records the external Oracle validation path used for the XA
multi-database delivery branch. It is intentionally kept as a guide instead of
an in-repository integration test because Seata Go does not add an Oracle driver
dependency. Applications inject their selected Oracle driver through
`RegisterSeataXADriver`.

## Scope

The validation path covers:

- Oracle Free `23.26.2.0.0` from `gvenzl/oracle-free:23-slim-faststart`.
- `github.com/sijms/go-ora/v2 v2.9.0` as the externally supplied Go driver.
- `RegisterSeataXADriver` wrapping an application-provided Oracle driver.
- `XAConn.BeginTx` opening the driver-level transaction before `DBMS_XA.XA_START`.
- `XATx.Commit` completing `XA_END + XA_PREPARE` and reporting phase one.
- Phase-two commit through the held XA connection.
- Direct resource checks for rollback, recovery visibility and cleanup, and
  readonly prepare returning `XA_RDONLY`.

## Database Setup

Run Oracle Free locally and create a throwaway application user. Do not commit
real passwords, wallet paths, hostnames, or full DSNs.

```bash
docker run -d --name seata-go-oracle-xa-it --rm \
  -e ORACLE_PASSWORD='<temporary-admin-password>' \
  -p 127.0.0.1:1521:1521 \
  gvenzl/oracle-free:23-slim-faststart
```

Grant `DBMS_XA` privileges from a privileged container session. In a CDB/PDB
layout, switch to the target PDB before granting privileges:

```sql
alter session set container=FREEPDB1;
grant execute on sys.dbms_xa to seata_xa;
grant select on sys.dba_pending_transactions to seata_xa;
grant select on sys.pending_trans$ to seata_xa;
```

Some Oracle Free setups do not allow `SYSTEM` to grant `SYS.DBMS_XA` directly.
Use a DBA-level session for the grant when `ORA-01031` is returned during setup.

## External Probe Module

Create the probe outside this repository so the Oracle driver remains an
external validation dependency:

```bash
probe_dir="$(mktemp -d /tmp/seata-oracle-xa-wrapper-probe.XXXXXX)"
cd "$probe_dir"
go mod init seata-oracle-xa-wrapper-probe
go mod edit -replace seata.apache.org/seata-go/v2=/path/to/incubator-seata-go
go get seata.apache.org/seata-go/v2
go get github.com/sijms/go-ora/v2@v2.9.0
```

The probe should:

1. Import go-ora only in the external module.
2. Register a `seata-xa-oracle` driver with `RegisterSeataXADriver`.
3. Return a stable resource name from `ParseDBName` without logging the DSN.
4. Open `sql.DB` with the wrapped driver name.
5. Create a small validation table.
6. Start a global transaction context and call `db.BeginTx`.
7. Execute one insert through the returned transaction.
8. Call `tx.Commit` and assert that phase-one reporting happened.
9. Complete phase two through the XA resource manager or equivalent held
   connection path.
10. Verify the inserted row is visible only after phase two.

For direct resource checks, the same external module can obtain a raw driver
connection, create an `OracleXAConn`, and validate prepare/commit,
prepare/rollback, `Recover(TMStartRScan|TMEndRScan)`, and readonly prepare.

## Expected Result

The wrapper probe used for this branch printed:

```text
oracle_xa_wrapper_probe=pass branch_id=1 reports=1
```

The direct resource probe confirmed:

- `DBMS_XA.XA_START` succeeds with `TMNOFLAGS`.
- Prepared commit makes the row visible after `XA_COMMIT`.
- Prepared rollback leaves no committed row.
- `DBMS_XA.XA_RECOVER()` sees prepared branches before phase two and no longer
  returns them after cleanup.
- Readonly prepare returns `XA_RDONLY`.
- The wrapper path no longer triggers `ORA-02089: COMMIT is not allowed in a
  subordinate session`.

## Validation Record

Record these facts with every real Oracle run:

- Seata Go branch and commit.
- Go version.
- Oracle image, edition, version, PDB or service name.
- Go Oracle driver name and version.
- Privileges granted to the application user.
- Whether `DBMS_XA.XA_START`, `XA_END`, `XA_PREPARE`, `XA_COMMIT`,
  `XA_ROLLBACK`, `XA_RECOVER`, and `XA_FORGET` were checked.
- Returned errors for duplicate phase-two callbacks, branch missing, invalid
  XID, insufficient privileges, and connection loss if they were exercised.

## Cleanup

```bash
docker rm -f seata-go-oracle-xa-it
rm -rf "$probe_dir"
```

