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

# Oracle XA 使用指南

Seata Go 的 Oracle XA resource 通过 Oracle `DBMS_XA` PL/SQL 调用实现，不使用 MySQL 风格的
`XA START` 控制语句。

Oracle 支持通过厂商 XA adapter 注册入口暴露。应用侧提供 Oracle `database/sql/driver.Driver`，
Seata Go 负责 `types.DBTypeOracle` 对应的 XA 分支生命周期。

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

`parseOracleServiceName` 应返回应用用于标识资源的 Oracle service name、数据库名或 schema 名。
不要在日志中输出完整 DSN，因为 Oracle 连接串经常包含用户名、密码或 wallet 配置。

## XA 映射

| Seata Go 方法 | Oracle 调用 |
| --- | --- |
| `Start` | `DBMS_XA.XA_START` |
| `End` | `DBMS_XA.XA_END` |
| `XAPrepare` | `DBMS_XA.XA_PREPARE` |
| `Commit` | `DBMS_XA.XA_COMMIT` |
| `Rollback` | `DBMS_XA.XA_ROLLBACK` |
| `Recover` | `TABLE(DBMS_XA.XA_RECOVER())` |
| `Forget` | `DBMS_XA.XA_FORGET` |

Oracle XID 映射到 `DBMS_XA_XID(formatid, gtrid, bqual)`：

- `formatid`：固定为 `9752`，对齐 Seata Java `XABranchXid`。
- `gtrid`：Seata 全局 XID 字节。
- `bqual`：Seata branch qualifier 字节，编码为 `-<branchId>`。
- `gtrid` 和 `bqual` 都必须满足 Oracle `RAW(64)` 限制。

Seata Go 会拒绝无法从 Oracle recover 结果反解的 XID，不会静默 hash 或截断超长 XID。

## 使用要求

- Oracle driver 需要实现 `database/sql/driver.ExecerContext`，用于执行生命周期 PL/SQL block。
- Oracle driver 需要实现 `database/sql/driver.QueryerContext`，用于执行恢复扫描。
- 应用用户需要具备执行 `DBMS_XA` 包的权限。
- 正式使用前，应确认应用能调用 `DBMS_XA.XA_RECOVER()`，或具备等价恢复路径。
- 验证失败后要确保 prepared branch 可见并被清理。

权限语句示例：

```sql
GRANT EXECUTE ON DBMS_XA TO seata_user;
```

具体授权方式可能受 Oracle 版本、CDB/PDB 布局和安全策略影响，需要由 DBA 结合实际环境确认。

## 本地测试

Oracle 单元测试使用 mock `driver.Conn`，不需要真实 Oracle 数据库：

```bash
go test ./pkg/datasource/sql/xa -run Oracle -v
```

完整 SQL 包测试：

```bash
go test ./pkg/datasource/sql/...
```

当前仓库还没有默认启用的 Oracle 集成测试。真实 Oracle 验证记录至少应包含：

- Oracle edition 和版本。
- Go driver 名称和版本。
- 应用用户权限。
- `DBMS_XA.XA_START`、`XA_END`、`XA_PREPARE`、`XA_COMMIT`、`XA_ROLLBACK`、
  `XA_RECOVER`、`XA_FORGET` 的实际行为。
- branch missing、重复二阶段回调、非法 XID、权限不足对应的错误。
- prepared branch 在连接关闭后是否仍可由 recovery connection 完成二阶段。

## 已知行为

| 行为 | 说明 |
| --- | --- |
| `Start` 的 `TMNoFlags` | 发送为 Oracle loose branch flag，用于对齐 Seata Java 行为。 |
| `End` 的 `TMFail` | 内部映射为 `TMSuccess` 后再 rollback，因为当前 `DBMS_XA.XA_END` 路径不接受 `TMFAIL`。 |
| `XAPrepare` 只读结果 | `DBMS_XA.XA_OK` 和 `DBMS_XA.XA_RDONLY` 都视为 prepare 成功。 |
| 恢复扫描 | 使用 `TABLE(DBMS_XA.XA_RECOVER())`，再把 `formatid/gtrid/bqual` 转回 Seata branch XID 字符串。 |
| already-ended 分类 | `ORA-24756`、`ORA-24761`、`XAER_NOTA` 和包装后的 `DBMS_XA` 数字返回码 `-4` 会被识别为 already-ended 状态。 |

## 故障排查

| 现象 | 检查项 |
| --- | --- |
| `oracle xa requires driver.ExecerContext` | 当前 Oracle driver 不能通过 `driver.ExecerContext` 执行 DBMS_XA 生命周期 PL/SQL block。 |
| `oracle xa recover requires driver.QueryerContext` | 当前 Oracle driver 不能通过 `driver.QueryerContext` 执行恢复查询。 |
| `ORA-01031: insufficient privileges` | 授权应用用户访问 `DBMS_XA`，或使用具备对应 package 权限的用户验证。 |
| `oracle xa gtrid exceeds RAW(64)` 或 `oracle xa bqual exceeds RAW(64)` | 缩短全局 XID，或等待项目接受新的 XID 映射策略。 |
| recover 返回的 branch qualifier 无法解析 | 确认所有参与应用使用相同的 Seata XID 映射和 `formatid=9752`。 |
