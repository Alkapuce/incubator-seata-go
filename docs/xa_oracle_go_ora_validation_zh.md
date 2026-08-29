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

# Oracle go-ora 验证记录

本文记录 XA 多数据库交付分支使用的 Oracle 外部验证路径。这里保留为验证指南，而不是仓库内默认
integration test，因为 Seata Go 不直接引入 Oracle driver 依赖。应用通过 `RegisterSeataXADriver`
注入自己选择的 Oracle driver。

## 覆盖范围

该验证路径覆盖：

- `gvenzl/oracle-free:23-slim-faststart` 提供的 Oracle Free `23.26.2.0.0`。
- 外部提供的 Go driver：`github.com/sijms/go-ora/v2 v2.9.0`。
- 通过 `RegisterSeataXADriver` 包装应用侧 Oracle driver。
- `XAConn.BeginTx` 在 `DBMS_XA.XA_START` 前打开 driver-level transaction。
- `XATx.Commit` 完成 `XA_END + XA_PREPARE` 并上报一阶段结果。
- 通过 held XA connection 完成二阶段提交。
- 直接 resource 检查 rollback、recover 可见性与清理，以及 readonly prepare 返回 `XA_RDONLY`。

## 数据库准备

本地启动 Oracle Free，并创建一次性应用用户。不要提交真实密码、wallet 路径、主机名或完整 DSN。

```bash
docker run -d --name seata-go-oracle-xa-it --rm \
  -e ORACLE_PASSWORD='<temporary-admin-password>' \
  -p 127.0.0.1:1521:1521 \
  gvenzl/oracle-free:23-slim-faststart
```

从具备权限的容器会话授予 `DBMS_XA` 权限。CDB/PDB 布局下，先切到目标 PDB：

```sql
alter session set container=FREEPDB1;
grant execute on sys.dbms_xa to seata_xa;
grant select on sys.dba_pending_transactions to seata_xa;
grant select on sys.pending_trans$ to seata_xa;
```

部分 Oracle Free 环境中，`SYSTEM` 直接授予 `SYS.DBMS_XA` 会返回 `ORA-01031`。遇到该错误时，使用
DBA 级会话完成授权。

## 外部 Probe 模块

在仓库外创建 probe，避免把 Oracle driver 变成项目依赖：

```bash
probe_dir="$(mktemp -d /tmp/seata-oracle-xa-wrapper-probe.XXXXXX)"
cd "$probe_dir"
go mod init seata-oracle-xa-wrapper-probe
go mod edit -replace seata.apache.org/seata-go/v2=/path/to/incubator-seata-go
go get seata.apache.org/seata-go/v2
go get github.com/sijms/go-ora/v2@v2.9.0
```

probe 应执行以下检查：

1. 只在外部模块中 import go-ora。
2. 使用 `RegisterSeataXADriver` 注册 `seata-xa-oracle` driver。
3. `ParseDBName` 返回稳定 resource name，不记录完整 DSN。
4. 使用包装后的 driver name 打开 `sql.DB`。
5. 创建小型验证表。
6. 启动全局事务上下文并调用 `db.BeginTx`。
7. 通过返回的事务执行一条 insert。
8. 调用 `tx.Commit`，确认一阶段上报已发生。
9. 通过 XA resource manager 或等价 held connection 路径完成二阶段。
10. 确认插入行只在二阶段完成后可见。

直接 resource 检查可以在同一个外部模块中获取 raw driver connection，创建 `OracleXAConn`，再验证
prepare/commit、prepare/rollback、`Recover(TMStartRScan|TMEndRScan)` 和 readonly prepare。

## 预期结果

本分支使用的 wrapper probe 输出为：

```text
oracle_xa_wrapper_probe=pass branch_id=1 reports=1
```

直接 resource probe 已确认：

- `DBMS_XA.XA_START` 使用 `TMNOFLAGS` 可以成功。
- prepare 后 commit 会在 `XA_COMMIT` 后让业务行可见。
- prepare 后 rollback 不会提交业务行。
- `DBMS_XA.XA_RECOVER()` 在二阶段前能看到 prepared branch，清理后不再返回。
- readonly prepare 返回 `XA_RDONLY`。
- wrapper 路径不再触发 `ORA-02089: COMMIT is not allowed in a subordinate session`。

## 验证记录

每次真实 Oracle 验证至少记录：

- Seata Go 分支和 commit。
- Go 版本。
- Oracle 镜像、edition、版本、PDB 或 service name。
- Go Oracle driver 名称和版本。
- 应用用户获得的权限。
- 是否检查 `DBMS_XA.XA_START`、`XA_END`、`XA_PREPARE`、`XA_COMMIT`、
  `XA_ROLLBACK`、`XA_RECOVER` 和 `XA_FORGET`。
- 如果覆盖了重复二阶段回调、branch missing、非法 XID、权限不足和连接断开，需要记录对应错误。

## 清理

```bash
docker rm -f seata-go-oracle-xa-it
rm -rf "$probe_dir"
```
