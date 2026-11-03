# 【PRD功能评审报告】

| 项目 | 内容 |
|---|---|
| 评审对象 | 《BOE US RMA 收发管理系统 PRD V1.7》 |
| 评审维度 | 需求完整性 / 自洽性 / 边界覆盖 / 权限校验 / 一期二期边界 |
| 评审人 | WMS/RMA 产品专家（智能体） |
| 评审日期 | 2026-11-03 |
| 评审原则 | **仅抛问题，不修改业务规则、不给替代方案** |

---

## 1. 冲突点（高优先级，必须先闭环）

| 序号 | PRD位置 | 矛盾描述 | 影响范围 | 建议（仅指向待闭环点） |
|---|---|---|---|---|
| C-01 | 5.5 三段式表 vs 6.2⑬/E12 | 三段式"原片处置"列使用 `SCRAPPED`（SCRAP 原片报废）、`TRANSFERRED_RC`（TRANSFER_RC 原片发维修中心），但 `original_disposition` 枚举⑬仅有 `ORIGINAL_SHIPPED` / `RETAINED` 两个值 | 出库/还货记录原片处置字段无法落库 SCRAP/TRANSFER_RC 场景 | 需闭环：这两类原片处置的枚举归属 |
| C-02 | 4.2 SN状态机 / E4.status / 6.2⑭ | RETAINED 定义为"待处置中间态（非终态）"，但状态机将 JUDGED→RETAINED 画为终态；且 RT-09 的 4 条处置结果（SCRAPPED/RETURN_VENDOR/REPAIR_TO_BUFFER/LONG_RETAIN）属于 `retained_disposition`（return_record 字段），并非 `sn_status` 枚举值——原片处置完成后 SN 状态如何变化无定义 | 原片处置结果无法反映到 SN 状态机 | 需闭环：RETAINED 处置后 sn_status 的取值 |
| C-03 | 5.5 三段式表 / 2.2 / 6.2③ / E7 | SCRAP、TRANSFER_RC 场景：原片需"报废出库(SCRAP_DISPOSAL)"或"发维修中心(SHIP_TO_RC)"，同时客户侧还要"buffer 件还货出库(SHIP_TO_CUSTOMER)"，即一个 SN 可能产生两个出库动作；但三段式表仅列 buffer 件出库，原片处置出库单无落点 | 原片报废/转中心出库动作无对应出库单模型 | 需闭环：SCRAP/TRANSFER_RC 是否产生两个出库单 |
| C-04 | RC-05 / E3-B / OB-10 / E7-B | 收货箱号与发货箱号编码规则相同（`BOE+YYMMDD+三位流水`）且均"独立序列"——两套独立序列会生成完全相同的箱号（如都出现 BOE261103001），无法区分收货箱/发货箱 | 箱号唯一性、扫码、箱标打印、追溯冲突 | 需闭环：是否允许重号、如何区分两类箱号 |
| C-05 | RT-07 / E1.closed_at / 4.2-B | TAT 定义为"close 发货完成时间 − 完整收货时间"，E1.closed_at 注释为"RMA close 发货完成"；但 RMA 状态机中 RETURNED→CLOSED 是"关闭操作"，发货完成(shipped_at)发生在 RETURNED 之前，close 时间 ≠ 发货完成时间 | TAT 算法取数歧义，报表口径不一致 | 需闭环：TAT 终点取 shipped_at 还是 closed_at |
| C-06 | SN-05 / E4 | SN-05 要求多收实物新建 sn_item 时 `source=UNFORECASTED`，但 E4 sn_item 字段清单中无 `source` 字段 | 多收实物来源标识无法落库 | 需闭环：source 字段归属 |
| C-07 | 8 非功能"多租户预留" vs E3-B/E5/E6/E7/E7-B/E8/E9/E11/E12 | 非功能声明"全核心表含 tenant_id"，但上述 9 个实体字段表中均无 `tenant_id` | 多租户隔离在数据层不完整，后续欧洲扩展需返工 | 需闭环：tenant_id 覆盖范围 |
| C-08 | 3 权限矩阵"差异双异常记录查看"行 | 客户/BOE US 对该行标 👁（只读），但双异常记录含项目人员与客户的沟通/审批说明（E5 approval_note） | 客户可见内部沟通信息，存在信息越权风险 | 需闭环：客户是否可见双异常记录 |
| C-09 | 8 非功能"部署形态" / 3章① / 10.2 | 非功能需求写"客户/BOE US 可远程只读访问轨迹（独立门户二期交付）"——一期出现"客户远程只读访问"能力，但独立门户列在二期 | 二期能力表述混入一期 | 需闭环：一期客户访问轨迹的方式 |

---

## 2. 模糊 / 缺口点（中优先级，需业务确认）

| 序号 | PRD位置 | 缺口描述 | 待确认问题 |
|---|---|---|---|
| M-01 | RT-05 / 3 权限矩阵 | RMA 关闭(RETURNED→CLOSED)操作人未定义，权限矩阵无"RMA 关闭"行 | 谁执行 RMA 关闭？ |
| M-02 | RT-01 / P-09 / 3 权限矩阵 | "还货方式确定"的操作人、界面、时机无明确；权限矩阵无此项 | 谁在哪个页面、何时确定还货方式？ |
| M-03 | BS-08 | 不同 PN buffer 替代"须项目人员审批"，但无审批记录实体、无审批状态、无审批界面 | 审批记录存哪、状态如何流转？ |
| M-04 | E11 / E4 / BS-04 | buffer 备件 SN 是否纳入 sn_item 状态机？buffer 件出库后如何追踪 | buffer 件 SN 与 sn_item 的关系 |
| M-05 | 4.2-B RMA状态机 | PROCESSING→PENDING_RETURN 的触发条件未定义（所有 SN 判定完成？含 CLOSED_SHORT 少收 SN？） | 该推进的触发条件 |
| M-06 | D7（4.2-B 注） | "所有批次 kitting 完成"判定：批次总数在货物到齐前未知，如何判定"所有批次" | 批次数如何确定 |
| M-07 | E5 / DC-09 | 一个少收差异可能对应多个预报 SN，E5 差异记录 sn 字段可为空，如何关联具体哪些 SN 被关单 | 少收与具体 SN 的关联方式 |
| M-08 | 5.5 三段式 / BS-06 | REPAIR 有"原片还客户"与"入 buffer 复用"两条路径，原片状态是先 RETAINED 再入 buffer，还是修好直接入 buffer 不经 RETAINED | REPAIR 两路径的状态时序 |
| M-09 | E12 / E7 / E7-B | return_record 无 outbound_no / shipment_box_no 关联字段，还货如何与出库单、发货箱挂接 | 还货记录与出库/发货箱的关联 |
| M-10 | E7.status / OB-04 | 出库单状态仅 CREATED/PENDING_APPROVAL/SHIPPED，调拨(TRANSFER_WAREHOUSE)的状态流转未定义 | 调拨出库单状态如何流转 |
| M-11 | E7-B | 发货箱 rma_no 为单值，一个发货箱是否可能装多个 RMA 的 SN；出库单是否按 RMA 拆分 | 发货箱与 RMA/出库单的归属规则 |
| M-12 | 3 权限矩阵 | 缺"BUFFER 库存管理（BS-01 入库）""收货箱标打印""发货打包/箱标打印/发运提交""RETAINED 处置操作""TAT 看板查看"等权限行 | 这些操作的权限归属 |
| M-13 | PH-03 | "照片保留≥2年"是否覆盖 POD 照片、箱标打印记录、发货箱相关图片 | 长期留存范围 |
| M-14 | E9.action | 审计日志 action 仅定义"受控修正(CONTROLLED_CORRECTION)"，未覆盖还货方式变更、RETAINED 处置、BUFFER 替代审批、RMA 关闭等关键操作 | 审计覆盖范围 |

> **M-01~M-14 已全部 grill 确认闭环（PRD V1.11）**，结论见 PRD V1.11 口径确认记录。

---

## 3. 遗漏边界场景（低优先级，补充说明）

| 序号 | 场景 | 说明 |
|---|---|---|
| L-01 | RETAINED 未处置完时二次返修 | SN-02 要求"上一轮生命周期已完结"，RETAINED 非终态，能否二次返修未定义 |
| L-02 | RETAINED 原片参与调拨 | OB-04 未限定调拨对象状态，RETAINED/CLOSED_SHORT 等状态能否调拨未定义 |
| L-03 | buffer 备件自身报废/二次返修 | buffer 件 OUT 后若退回、或本身需报废，路径未覆盖 |
| L-04 | 少收对 TAT / RMA close 的影响 | TAT 按"所有批次允收完成"起算，少收批次的处理对起算点影响未说明 |
| L-05 | 多收 SN 回填 RMA 后的还货与 TAT 影响 | 多收实物回填 RMA/客户后是否计入还货进度与 TAT，未说明 |
| L-06 | 一 RMA 多 PN 场景 | FC-01 预报单含单一料号；若一个 RMA 跨多 PN，buffer 同 PN 匹配规则（BS-08）如何适配 |
| L-07 | 部分 SN 已出库、部分 RETAINED 时 RMA 推进 | RMA 状态聚合在混合状态下的判定 |
| L-08 | 报废审批驳回路径 | OB-03 只定义审批通过留痕，无驳回/退回状态与流转 |

> **L-01~L-08 已全部 grill 确认闭环（PRD V1.12）**。至此本报告 C-01~C-09、M-01~M-14、L-01~L-08 全部闭环。

---

## 4. 合规 & 数据安全校验

| 检查项 | 现状 | 结论 |
|---|---|---|
| 双异常记录不可删 | DC-05 / E5 注：任何角色不得删除 | ✅ 合规 |
| 受控修正审计留痕 | E9 审计日志、KT-05 | ⚠️ 仅覆盖"受控修正"，未覆盖还货方式变更/RETAINED 处置/BUFFER 替代审批/RMA 关闭（见 M-14） |
| 照片留存 | PH-03 ≥2 年；E8.retention_until | ⚠️ POD 照片/箱标/发货箱图片是否纳入未明（见 M-13） |
| 客户数据可见边界 | 客户可见"差异双异常记录"（C-08）、发货箱物流信息（E7-B carrier/tracking_no） | ⚠️ 存在内部信息对客户暴露风险 |
| SN 全局唯一 | FC-03/SN-01 tenant+sn 唯一索引 | ✅ 合规 |
| 逻辑删除/不可删 | 双异常记录不可删 ✅；其余业务单据是否可删未定义 | ⚠️ 需明确其他单据删除策略 |

---

## 5. 评审自检清单（逐条核对）

### 5.1 SN 状态机

| 核对项 | 结果 |
|---|---|
| 状态枚举 9 值（FORECASTED/RECEIVED/DISCREPANCY/INSPECTING/JUDGED/OUTBOUND/COMPLETED/CLOSED_SHORT/RETAINED） | ⚠️ RETAINED 为中间态却画成终态（C-02） |
| 流转完整（预报→接收→质检→判定→出库→完成） | ✅ 主链完整 |
| 差异回流 DISCREPANCY→RECEIVED | ✅ |
| 少收 FORECASTED→CLOSED_SHORT | ✅，但 CLOSED_SHORT 对 RMA 推进影响未明（M-05） |
| BUFFER JUDGED→RETAINED | ⚠️ RETAINED 处置后状态无落点（C-02） |
| buffer 件 SN 是否入状态机 | ❌ 未定义（M-04） |
| 二次返修 COMPLETED→新一轮 | ⚠️ RETAINED 未处置时能否返修（L-01） |

### 5.2 RMA 状态机

| 核对项 | 结果 |
|---|---|
| 6 态 OPEN→RECEIVING→PROCESSING→PENDING_RETURN→RETURNED→CLOSED | ✅ |
| RECEIVING→PROCESSING（所有批次 kitting 完成） | ⚠️ "所有批次"判定不可知（M-06） |
| PROCESSING→PENDING_RETURN（判定完成） | ❌ 触发条件缺失（M-05） |
| PENDING_RETURN→RETURNED（所有 SN 还货） | ✅ 含 CLOSED_SHORT/RETAINED |
| RETURNED→CLOSED 操作人 | ❌ 缺失（M-01） |

### 5.3 还货三段映射

| 核对项 | 结果 |
|---|---|
| REPAIR→ORIGINAL_REPAIR/BUFFER_RETURN | ⚠️ 两路径状态时序不清（M-08） |
| REPLACE→BUFFER_RETURN（非原片还货） | ✅ |
| SCRAP→原片 SCRAPPED + buffer 件还货 | ❌ 原片处置枚举缺值（C-01）+ 双出库未建模（C-03） |
| TRANSFER_RC→原片 TRANSFERRED_RC + buffer 件还货 | ❌ 同上（C-01/C-03） |
| CID→CID_RETURN 原片还 | ✅ |
| 原片处置枚举完整性 | ❌ 缺 SCRAPPED/TRANSFERRED_RC（C-01） |

### 5.4 箱号规则

| 核对项 | 结果 |
|---|---|
| 收货箱号 BOE+YYMMDD+三位流水、按箱递增、≤999 | ✅ |
| 发货箱号同规则、独立序列 | ❌ 与收货箱号可重号（C-04） |
| 箱标打印（收货=箱号+SN；发货=还货RMA+箱号+SN+重量+材积） | ✅（打印权限缺失见 M-12） |

### 5.5 TAT 规则

| 核对项 | 结果 |
|---|---|
| 定义 = close 发货完成 − 完整收货 | ❌ 终点取数歧义（C-05） |
| 起点 = 所有批次允收完成 | ⚠️ 少收/多批次对起算影响（L-04） |
| ≤10 自然日 + 8 天预警 + 超期标红 | ✅ |
| TAT 报表（平均/分布/超期率） | ✅（RP-06） |

---

## 6. 结论

- **高优先级冲突 9 项（C-01~C-09）必须先闭环**，其中 C-01/C-02/C-03（原片处置与双出库）、C-04（箱号重号）、C-05（TAT 取数）直接影响数据模型与核心算法；
- 中优先级 14 项需业务确认；
- 全部问题仅列出，不修改业务规则、不给替代方案，等你终裁/澄清后我再按结论回写。

---

## 7. grill 确认结果（2026-11-03，老丁拍板）

| 序号 | 确认结论 | 状态 |
|---|---|---|
| C-05 | TAT 终点 = **RMA 完整发货完成时间**；close 时点与发货完成时点为**同一时间点** | ✅ 闭环 |
| C-03 | SCRAP、TRANSFER_RC 为**两个独立场景，各自对应自己的出库单**（SCRAP→SCRAP_DISPOSAL；TRANSFER_RC→SHIP_TO_RC） | ✅ 闭环（buffer 件还货出库单归属见 M-03 追踪） |
| C-04 | **不允许重号**；发货箱号编码 = **`OB + YYMMDD + 三位流水`**（前缀 OB，与收货箱 BOE 区分） | ✅ 闭环 |
| C-08 | 客户/BOE US **可见**差异双异常记录 | ✅ 闭环 |
| C-01 | 原片处置 4 值：原片发走 / 留库待定 / 报废(SCRAPPED) / 转维修中心(TRANSFERRED_RC) | ✅ 闭环（V1.9） |
| C-02 | sn_status 新增统一终态 **DISPOSED**；处置路径记 retained_disposition | ✅ 闭环（V1.9） |
| C-06 | 不加 source 字段，以 forecast_no 为空标识未预报实物 | ✅ 闭环（V1.9） |
| C-07 | 所有核心表补 tenant_id | ✅ 闭环（V1.9） |
| C-09 | 一期客户不访问系统，客户只读与门户全部二期 | ✅ 闭环（V1.9） |

> 高优先级冲突 C-01~C-09 已全部闭环；中优先级 M-01~M-14 仍待确认。
