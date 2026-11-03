# BOE US RMA 收发管理系统 — 一期 MVP

基于《RMA 收发系统 PRD V1.22》全量实现，技术栈：**Go 1.22 + Gin + GORM + MySQL 8 / Vue3 + Element Plus / MinIO / JWT RBAC**。
中英双语界面 · 列表分页 10/20/50/200 · 浏览器原生打印箱标 · Web 扫码枪（键盘模拟输入）· H5 移动端。

## 目录结构

```
mvp/
├── backend/                 Go 后端（分层：handler → service → statemachine/model）
│   ├── cmd/server/          入口
│   └── internal/
│       ├── enum/            枚举常量（唯一权威）
│       ├── model/           GORM 模型（E1~E12 + 序列表/审批表）
│       ├── statemachine/    SN 10 态 + RMA 6 态
│       ├── seq/             箱号 BOE/OB 双序列 + 通用单号
│       ├── service/         业务编排（事务/审计/状态机校验）
│       ├── handler/         HTTP 层
│       ├── middleware/       JWT + RBAC
│       ├── router/          路由与权限绑定
│       ├── storage/         对象存储（MinIO / 内存）
│       └── store/           连接/迁移/种子
├── frontend/                Vue3 SPA（13 个 Web 页 + 移动端页）
├── db/                      schema.sql（MySQL 8 DDL）+ enum_seed.sql
├── openapi/                 OpenAPI 3.0
├── deploy/                  Dockerfile ×2 / docker-compose / nginx / rollback.sh
└── docs/                    需求覆盖 / 风险台账 / 架构ER / 测试报告 / 迭代与门禁 / 审计 / 枚举
```

## 快速启动（本地开发，SQLite）

```bash
cd mvp/backend
go run ./cmd/server          # 默认 sqlite，端口 8080，自动建表+种子
```

```bash
cd mvp/frontend
npm install && npm run dev   # http://localhost:5173 （代理到 8080）
```

演示账号（首次启动自动创建）：

| 角色 | 账号 | 密码 | 权限范围 |
|---|---|---|---|
| 项目人员 | project | Proj@123 | 预报、差异审批、报废审批、跨PN审批、RETAINED 处置、报表、审计 |
| 仓库操作员 | warehouse | Wh@123 | 允收、kitting 扫码、差异提交、出库、打包发运、箱标打印 |
| 质检员/R&D | inspector | Insp@123 | 质检判定（含批量）、只读报表 |
| 系统管理员 | admin | Admin@123 | 主数据、用户角色、报表、审计 |
| 客户/BOE US | customer | Cust@123 | **一期不开放登录（403）** |

## 生产部署（MySQL + MinIO + Nginx）

```bash
cd mvp/deploy
cp .env.example .env            # 按需改密码/端口
docker compose up -d --build
# 访问 http://localhost:8088
```

`db/schema.sql` 与 `db/enum_seed.sql` 会在 MySQL 容器首次启动时自动挂载执行。

回滚：

```bash
cd mvp/deploy && ./rollback.sh 1.0.0     # 指定历史 tag
```

## 环境变量

| 变量 | 默认 | 说明 |
|---|---|---|
| APP_PORT | 8080 | 服务端口 |
| DB_DIALECT | sqlite | `mysql` / `sqlite` |
| DB_DSN | rma.db | MySQL: `user:pwd@tcp(host:3306)/db?charset=utf8mb4&parseTime=True&loc=Local` |
| TENANT_ID | BOE_US | 一期单租户 |
| JWT_SECRET | dev 默认值 | **生产必须替换** |
| MINIO_ENDPOINT | 空 | 空则用内存存储（仅开发） |
| MINIO_ACCESS_KEY / MINIO_SECRET_KEY / MINIO_BUCKET | minioadmin / rma-photos | 对象存储 |
| MINIO_USE_SSL | false | — |

## 测试

```bash
cd mvp/backend
go test ./...            # 32 个用例：状态机 / 箱号 / 业务闭环 / 接口权限
go vet ./...
```

## 关键业务口径（不可改）

1. **两套状态机**：SN 10 态、RMA 6 态，逐级推进，禁跳级/回退；`COMPLETED`=正常发客户/调拨，`DISPOSED`=处置终态，`RETAINED`=中间态，`CLOSED_SHORT`=终态。
2. **三段式**：判定 → 自动带出还货方式 → 出库对象；BUFFER 默认同 PN，跨 PN 须项目人员审批；预占 `RESERVED` 防并发。
3. **箱号隔离**：收货箱 `BOE+YYMMDD+3位`（kitting 开箱生成）；发货箱 `OB+YYMMDD+3位`（打包生成）；两套独立序列，单日 ≤999，跨日重置。
4. **差异两类**：RECEIVING 差异审批通过仅放行；KITTING 差异审批通过 → 预报 `REVISED`；双异常记录永久不可删。
5. **RMA 自动关闭**：无手工按钮；`return_record(SHIPPED)` 数 = 需还货 SN 数即自动 `RETURNED→CLOSED`；RETAINED 不阻断。
6. **TAT**：完整收货 → 完整发货完成 ≤10 自然日；第 8 天预警、>10 标红。
7. **二次返修**：仅 `COMPLETED` 可返修；历史轮次归档 `sn_item_round`，`return_count+1`。
8. **一期不做**：客户门户、第三方 API、离线缓存、CANCELLED 作废态。

## 已知限制

见 `docs/02_风险台账.md`（R-016 本机无 MySQL/Docker 未实跑、R-019 无浏览器 E2E）与 `docs/05_迭代报告与门禁.md`。
