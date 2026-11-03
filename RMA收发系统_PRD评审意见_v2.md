# BOE US RMA 收发管理系统 PRD 评审意见 V2（V1.4 专项）

| 项目 | 内容 |
|---|---|
| 评审对象 | 《BOE US RMA 收发管理系统 PRD V1.4》 |
| 评审范围 | V1.4 新增：还货闭环、BUFFER 还货、RMA 状态机、RETAINED、BUFFER 库存管理 |
| 评审人 | WMS/RMA 产品专家（智能体） |
| 终裁 | 老丁（最终裁判官） |
| 评审日期 | 2026-11-03 |
| 关联 | V1 评审意见（针对 V1.2，已闭环） |

---

## 一、评审结论概览

**总体结论：有条件通过 ⚠️（存在 1 个设计冲突必须终裁后才能冻结 V1.5）**

- 还货闭环框架（RMA close=还货完成）、RMA 状态机 6 态、还货方式 3 类、BUFFER 库存管理整体方向成立；
- 但 **"判定→出库映射"与"BUFFER 还货"存在设计冲突**（见第三章 D-关键），必须终裁重构后才能研发落地；
- 另有 5 个决策点需老丁拍板（D6~D10）。

---

## 二、V1.4 新增内容核查

| 新增项 | 评审结论 | 备注 |
|---|---|---|
| RMA 状态机（6 态） | ✅ 方向通过 | 状态聚合驱动规则需补（见 D7） |
| 还货方式 3 类（SN 级） | ✅ 通过 | 确定时机需明确（见 D-关键） |
| RMA close = 所有 SN 还货完成 | ⚠️ 见 D8 | SCRAP/TRANSFER_RC 的闭环计数需明确 |
| BUFFER 还货 + 原片 RETAINED | ⚠️ 见 D-关键 + D9 | 与判定→出库映射冲突；RETAINED 非真终态 |
| buffer_stock 实体 + 库存校验 | ✅ 通过 | 维修良品入 buffer 路径需补（见 D10） |
| return_record（original_sn + returned_sn） | ✅ 通过 | 客户对账 SN 口径需说明 |
| 新增枚举 4 组 | ✅ 通过 | 可直接落地 |

---

## 三、关键设计冲突（必须终裁）

### D-关键：判定→出库映射 与 BUFFER 还货 矛盾

**冲突描述**：
- PRD 5.5 判定→出库映射表规定：判定 `REPAIR` → 出库 `SHIP_TO_CUSTOMER`（**原片出库**）；
- 但 PRD 5.11 RT-03 规定：还货方式 `BUFFER_RETURN` 时，**原片不出库（RETAINED）**，改由 buffer 件出库；
- 而 RT-01 未明确还货方式的**确定时机**——若按"判定→出库→再定还货方式"的顺序，则原片已按映射表出库，与 BUFFER 还货（原片留库）直接冲突，系统无法同时满足。

**根因**：出库对象（原片 vs buffer 件）取决于还货方式，但还货方式在现有流程中位于出库之后，时序倒置。

**专家建议（推荐方案）**：将"还货方式确定"**前移至判定之后、出库之前**，重构为三段式：
```
判定结果 → 确定还货方式 → 决定出库对象
  REPAIR   → ORIGINAL_REPAIR → 原片出库 SHIP_TO_CUSTOMER
  REPAIR   → BUFFER_RETURN   → buffer 件出库，原片 RETAINED
  CID_RETURN→ CID_RETURN     → 原片出库 SHIP_TO_CUSTOMER(cid_flag)
```
即：判定 REPAIR 后，由项目人员/规则选择"原片修复还货"或"BUFFER 还货"，再决定出库对象。判定→出库映射表需据此重构。

**需老丁终裁**：是否采用"判定→还货方式→出库对象"三段式？还是其他方案？

---

## 四、待老丁终裁的决策点（D6~D10）

| 编号 | 决策点 | 专家建议 | 需老丁拍板 |
|---|---|---|---|
| D6 | **还货方式确定时机**（D-关键的具体落地） | 判定后、出库前确定；出库对象由还货方式决定 | 采用三段式重构？ |
| D7 | **多批次到货时 RMA RECEIVING→PROCESSING 触发条件** | 建议：该 RMA 所有批次 kitting 完成才推进 PROCESSING（而非首批） | 按全部批次 kitting 完成，还是首批？ |
| D8 | **SCRAP / TRANSFER_RC / REPLACE 的 SN 在 RMA close 中的计数** | 建议：SCRAP、TRANSFER_RC（转维修中心，货已发走）视为"已处置闭环"，计入 RMA close 闭环计数；REPLACE 归入"非原片还货"（原片处置另定） | 是否认可？REPLACE 归类？ |
| D9 | **RETAINED 原片的性质与后续处置** | RETAINED 应为"待处置中间态"而非终态；后续处置路径：报废 / 退供应商 / 转入 buffer 复用 / 留库 | 原片后续处置路径有哪些？ |
| D10 | **维修良品入 buffer 的路径与触发** | 明确：判定 REPAIR 修好后，可选择"原片还客户"或"入 buffer 复用"两条路径之一 | 维修良品入 buffer 的触发条件？ |

---

## 五、新发现的问题与建议

| 编号 | 问题 | 建议 |
|---|---|---|
| R-NEW-6 | RMA 状态由 SN 状态聚合驱动，但聚合规则未明确（如 PENDING_RETURN→RETURNED 的系统判定逻辑） | 补规则：RMA 状态由其下所有 SN 的还货/闭环状态聚合自动推进；RETURNED 需所有 SN 还货完成（含 SCRAP/TRANSFER_RC/RETAINED/CLOSED_SHORT 闭环） |
| R-NEW-7 | RMA close（RETURNED→CLOSED）的操作人未指定 | 建议：项目人员操作关闭，留痕审计 |
| R-NEW-8 | BUFFER 还货时客户对账 SN 口径：客户退的是 SN=X，还回的是 SN=Y（buffer 件），客户对账可能困惑 | 还货单据同时体现 original_sn 与 returned_sn，并向客户说明 BUFFER 替代 |
| R-NEW-9 | buffer_stock 入库未校验料号在主数据 | BS-01 增加料号主数据校验 |
| R-NEW-10 | RETAINED 原片长期堆积无清理机制 | 配套 RETAINED 超期提醒 + 处置工单（与 R-07 联动） |

---

## 六、评审产出建议

1. 老丁终裁 D-关键（D6）+ D7~D10 后，PRD 升版 **V1.5** 并重构"判定→还货方式→出库对象"三段式映射；
2. R-NEW-6~10 补规则纳入 V1.5；
3. V1.5 冻结后进入研发任务拆解；
4. 评审结论沉淀到事项 `rodU91`。

---

> **专家签字（智能体）**：V1.4 还货闭环与 BUFFER 框架方向正确，但"判定→出库映射"与 BUFFER 还货存在时序冲突，必须按 D-关键重构。请老丁终裁 D6~D10。
