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

# XA 多数据库交付说明

本文汇总 XA 多数据库支持的本地交付范围，以及提交 Apache PR 前需要完成的检查。

## 已实现范围

| 范围 | 状态 |
| --- | --- |
| MariaDB | 新增 `seata-xa-mariadb`、MariaDB XA resource factory、MySQL-compatible XA 生命周期语句、recover 解析、MariaDB 专属错误分类、XAConn autoCommit 覆盖、集成测试和用户文档。 |
| Oracle | 新增 Oracle `DBMS_XA` XID 映射、生命周期调用、recover 解析、prepared statement fallback、already-ended 错误分类、单元测试和配置/排查文档。 |
| 厂商 adapter | 新增 `RegisterSeataXADriver` 和 `SeataDriverDescriptor`，应用可以注册外部 `database/sql/driver.Driver`，无需把厂商 driver 加入 Seata Go 直接依赖。 |
| 达梦原型 | 新增 `types.DBTypeDM` 和基于 `DBMS_XA` 的 XA resource 原型，覆盖 XID 映射、生命周期调用、recover 解析、错误分类、单元测试和文档。 |
| Kingbase 与 Oscar | 已记录扩展方向和待确认问题。Kingbase 优先按 PostgreSQL prepared transaction 路径验证；Oscar 需要先确认公开 Go driver、XA API、recover 和错误码。 |

## 验证命令

提交或更新 PR 前执行：

```bash
git diff --check
go test ./pkg/datasource/sql/types -run 'DBType|ParseDBType|IndexConstants' -v
go test ./pkg/datasource/sql/xa -run 'MariaDB|Oracle|DM' -v
go test ./pkg/datasource/sql -run 'TestXAConn_ExecContext|TestXAConn_AutoCommit' -v
go test ./pkg/datasource/sql/xa ./pkg/datasource/sql/types
go test ./pkg/datasource/sql/...
go test ./...
```

具备 MariaDB DSN 时执行集成测试：

```bash
SEATA_GO_TEST_MARIADB_DSN='user:password@tcp(127.0.0.1:3306)/seata_demo?parseTime=true' \
  go test -tags integration ./pkg/datasource/sql/xa -run 'TestMariaDBXAConnIntegration' -v
```

## 合规检查

| 检查项 | 当前结果 |
| --- | --- |
| 新增依赖 | 当前实现不需要修改 `go.mod` 或 `go.sum`。 |
| 厂商 driver | Oracle 和达梦 driver 由应用通过厂商 adapter API 注入，没有加入项目直接依赖。 |
| License header | 新增 Go 和 Markdown 文件均包含 Apache Software Foundation license header。 |
| 生成文件 | `dbtype_string.go` 已随 DB type 测试同步更新。 |
| 敏感信息 | 文档示例使用占位值，不应包含真实 DSN、密码、wallet 或私有部署信息。 |

## 建议 PR 拆分

| PR | 范围 | 说明 |
| --- | --- | --- |
| PR 1 | Java/Go 对比、XA 契约说明和 MariaDB resource | 先让第一个新增数据库支持可独立评审，并包含 MariaDB 集成测试路径。 |
| PR 2 | Oracle XID helper 和 `DBMS_XA` 生命周期 | 聚焦 XID 编码、PL/SQL block 构造和生命周期映射。 |
| PR 3 | Oracle recover、classifier、fallback 和文档 | 把恢复语义、driver fallback 和用户配置说明放在一起评审。 |
| PR 4 | 厂商 XA adapter 注册入口 | 单独评审外部 driver 注入的公开 API。 |
| PR 5 | 达梦原型和国产数据库扩展说明 | 达梦在真实库验证前按 prototype 呈现。 |

如果维护者倾向更少 PR，可以重新 squash，但 PR 描述仍应保留这些边界。

## 剩余验证

| 优先级 | 事项 | 原因 |
| --- | --- | --- |
| P0 | Oracle 真实库验证 | 单元测试只能证明 Go 侧 PL/SQL 构造；真实 Oracle driver 和数据库还需要确认命名绑定、权限、recover 行和错误文本。 |
| P0 | 达梦 driver 与许可证验证 | 当前原型刻意不引入直接依赖，官方 driver 来源、版本、许可证和再分发条款明确前不能声明生产支持。 |
| P1 | 达梦真实库验证 | `DBMS_XA` 可用性、`XA_COMPATIBLE_MODE`、recover 形态和真实错误码需要实测。 |
| P1 | MariaDB 重复回调执行 | 集成测试套件已包含重复二阶段回调覆盖；广泛声明支持前仍需带真实 MariaDB DSN 执行。 |
| P2 | Kingbase 原型决策 | 基于 PostgreSQL prepared transaction 的原型可行性较高，但应等待 driver 和兼容性确认。 |
| P2 | Oscar 后续跟进 | 只有在公开 Go driver、XA API、recover 和错误码证据明确后再新增代码。 |

## 支持等级表述

release notes 和 PR 描述建议使用准确措辞：

- MariaDB：具备单元测试、文档和 MariaDB 集成测试路径的 XA resource 支持。
- Oracle：具备 mock 覆盖和配置文档的 `DBMS_XA` 实现，仍需真实数据库验证。
- 厂商 adapter：用于包装外部 driver 的公开扩展 API，不引入直接厂商依赖。
- 达梦：真实 driver 许可证、兼容模式、recover 输出和错误码验证前，只标记为 prototype resource。
