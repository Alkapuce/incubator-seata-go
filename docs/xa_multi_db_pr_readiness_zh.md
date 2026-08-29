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

# XA 多数据库 PR 准备清单

本文把 XA 多数据库支持的原始要求映射到当前本地实现、文档和验证证据，用于打开或更新 Apache
PR 前逐项核对。

## 原始要求到证据矩阵

| 原始要求 | 当前证据 | PR 表述 |
| --- | --- | --- |
| 对比 Seata Java 与 Seata Go 的 XA 数据库支持、控制语句、恢复机制和错误语义。 | `docs/xa_multi_db_delivery.md` 与 `docs/xa_multi_db_delivery_zh.md` 汇总 Go 侧交付范围；外层项目文档 `docs/seata-go-xa-java-go-db-support-comparison.md` 保留 Java/Go/数据库对比，作为评审准备资料。 | 已完成本地评审材料。若维护者偏好更短的仓库内说明，可在 PR 前移动或精简对比文档。 |
| 增加 MariaDB XA 支持。 | `pkg/datasource/sql/xa/mariadb_xa_connection.go`、`pkg/datasource/sql/driver.go`、`pkg/datasource/sql/types/types.go`、`docs/xa_mariadb.md`、`docs/xa_mariadb_zh.md` 和 `docs/quickstart*.md` 覆盖 driver name、DB type、生命周期、recover、classifier、使用方式和排查路径。本地 Docker MariaDB `11.4.13-MariaDB-ubu2404` 已验证 commit、rollback、recover 清理和重复二阶段回调。 | 具备单元测试、文档和 MariaDB 11.4.13 集成验证的 supported XA resource。 |
| 增加 Oracle XA 支持，并尽量对齐 Seata Java 的 `DBMS_XA` 路径。 | `pkg/datasource/sql/xa/oracle_xa_connection.go`、`oracle_xa_plsql.go`、`oracle_xid.go`、`docs/xa_oracle.md`、`docs/xa_oracle_zh.md` 和 `docs/xa_oracle_go_ora_validation*.md` 覆盖 XID 映射、`DBMS_XA` 生命周期、recover、readonly prepare、classifier、driver prepare fallback 和外部 driver 验证。Oracle Free `23.26.2.0.0` 加 `github.com/sijms/go-ora/v2 v2.9.0` 已验证 wrapper path 和 direct resource checks。 | 具备 mock 覆盖、配置文档、Oracle Free 加 go-ora 外部验证的 `DBMS_XA` 实现；生产支持口径前仍建议补充更完整的 driver / edition 验证。 |
| 提供可复用厂商 adapter 和国产数据库扩展路径，至少给出一个原型。 | `pkg/datasource/sql/driver.go` 中的 `RegisterSeataXADriver` 与 `SeataDriverDescriptor` 提供 driver 注入 API。`pkg/datasource/sql/xa/dm_xa_connection.go`、`dm_xa_plsql.go`、`dm_xid.go`、`docs/xa_dm*.md` 和 `docs/xa_dm_validation*.md` 提供达梦原型和真实验证清单。`pkg/datasource/sql/xa/xa_resource_contract_test.go` 为所有 supported XA DB type 固化 factory/resource/classifier 注册和共享 XA resource 行为。Kingbase 与 Oscar 保持在交付说明中的后续事项。 | 厂商 adapter 和共享契约测试可进入评审；达梦在真实 driver、许可证、兼容模式、recover 输出和错误码验证完成前只按 prototype 表述。 |
| 提供配置、权限、示例、排查、验证和 Apache 合规边界。 | `docs/quickstart*.md`、`docs/xa_mariadb*.md`、`docs/xa_oracle*.md`、`docs/xa_oracle_go_ora_validation*.md`、`docs/xa_vendor_adapter*.md`、`docs/xa_dm*.md` 和 `docs/xa_dm_validation*.md` 覆盖配置与支持等级表述。当前实现没有加入 Oracle 或达梦直接 driver 依赖，新增 Go/Markdown 文件均带 ASF license header。 | 可作为评审包提交。示例不写入敏感信息；RM branch-end processor 日志不输出 `applicationData` payload；外部 driver 依赖只有在许可证和再分发条款明确后再进入仓库。 |

## PR 前重跑命令

在仓库根目录执行：

```bash
git diff --check
go test ./pkg/datasource/sql/types -run 'DBType|ParseDBType|IndexConstants' -v
go test ./pkg/protocol/branch ./pkg/protocol/codec -run 'TestBranchStatus|TestBranchReportRequestCodec' -v
go test ./pkg/rm/remoting/grpc -run 'TestGetGrpcRMRemotingInstance|TestGrpcRMRemotingBranchRegisterXAType|TestGrpcRMRemotingBranchReportReadonlyStatus' -v
go test ./pkg/remoting/grpc ./pkg/remoting/processor/client
go test ./pkg/datasource/sql/xa -run 'TestXAResource' -v
go test ./pkg/datasource/sql/xa -run 'Postgres|MariaDB|Oracle|DM' -v
go test ./pkg/datasource/sql -run 'TestRegisterSeataXADriver|TestXAConn_BeginTx_DBMSXADriverTxHeldUntilPhaseTwo|TestXAConn_BeginTx_DBMSXAReadonlyPrepareReportsReadonlyAndReleases' -v
go test ./pkg/datasource/sql/xa ./pkg/datasource/sql/types
go test ./pkg/datasource/sql/...
go test ./...
```

具备本地 MariaDB DSN 时执行集成测试：

```bash
SEATA_GO_TEST_MARIADB_DSN='user:password@tcp(127.0.0.1:3306)/seata_demo?parseTime=true' \
  go test -count=1 -tags integration ./pkg/datasource/sql/xa -run 'TestMariaDBXAConnIntegration' -v
```

对拟提交 PR 范围执行 license header 扫描：

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

## 外部验证记录

PR 描述或关联验证文档中应记录：

- MariaDB image、server version、隐藏凭据后的 DSN 形态、测试命令和重复回调行为。
- Oracle image、edition、version、PDB 或 service name、外部 Go driver 名称和版本、授权项、
  wrapper probe 结果、direct resource checks，以及已覆盖的重复回调或连接断开错误。
- 达梦 driver 来源、版本、许可证、再分发条款、`database/sql` driver name、Go version、
  `XA_COMPATIBLE_MODE`、`DBMS_XA` 可用性、recover 返回形态和真实错误文本；这些信息齐备前不要改变支持等级。

## 阻塞与非阻塞事项

| 事项 | PR 影响 |
| --- | --- |
| MariaDB 生命周期、recover 清理和重复二阶段回调验证 | 不阻塞。当前已有本地 Docker 集成证据。 |
| Oracle Free 加 go-ora wrapper 和 direct `DBMS_XA` 验证 | 对 Oracle 实现 PR 不阻塞，但 PR 表述需要注明已测 driver 与 edition；更广的 Oracle driver / edition 验证作为后续事项。 |
| 达梦真实库和 driver 验证 | 阻塞 supported Dameng 表述；若代码和文档保持 prototype 口径，不阻塞原型 PR。 |
| Kingbase 与 Oscar 确认 | 不阻塞当前原型范围；driver、XA API、recover 和错误码证据明确前只保留为后续事项。 |

## 不应包含

- 真实 DSN、密码、wallet 路径、私有主机名或内部部署细节。
- 未确认来源、许可证和再分发条款的 Oracle 或达梦直接 driver 依赖。
- 缺少真实数据库、driver、recover 和错误语义证据的 Dameng、Kingbase 或 Oscar production-ready 表述。
- 临时 probe module、数据库 dump、build directory、原始 container log 等生成或环境相关产物。
