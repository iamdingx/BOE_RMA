-- ============================================================
-- 枚举字典 & 初始化数据（PRD V1.22 6.2 ①~⑭）
-- 说明：代码侧枚举以 Go 常量固化（internal/enum），本表用于前端下拉/报表字典/审计展示
-- ============================================================
SET NAMES utf8mb4;

CREATE TABLE IF NOT EXISTS enum_dict (
  enum_name  VARCHAR(48)  NOT NULL COMMENT '枚举名',
  enum_value VARCHAR(48)  NOT NULL COMMENT '值',
  label_zh   VARCHAR(64)  NOT NULL,
  label_en   VARCHAR(64)  NOT NULL,
  sort_no    INT          NOT NULL DEFAULT 0,
  remark     VARCHAR(255) NULL,
  PRIMARY KEY (enum_name, enum_value)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='枚举字典';

DELETE FROM enum_dict WHERE enum_name IN
 ('sn_status','judgment_result','outbound_type','discrepancy_type','anomaly_category','forecast_status',
  'approval_status','outbound_status','photo_biz_type','rma_status','return_method','buffer_stock_status',
  'original_disposition','retained_disposition','role','discrepancy_source');

-- ① SN 状态
INSERT INTO enum_dict VALUES
('sn_status','FORECASTED','已预报','Forecasted',1,'预报录入'),
('sn_status','RECEIVED','已接收','Received',2,'kitting 四维校验通过/差异闭环'),
('sn_status','DISCREPANCY','差异处理中','Discrepancy',3,'kitting 校验不通过'),
('sn_status','INSPECTING','质检中','Inspecting',4,'进入拆包质检'),
('sn_status','JUDGED','已判定','Judged',5,'判定结果录入'),
('sn_status','OUTBOUND','已出库','Outbound',6,'出库执行完成'),
('sn_status','COMPLETED','完成','Completed',7,'正常发客户/调拨完成'),
('sn_status','CLOSED_SHORT','少收关单','Closed (Short)',8,'终态'),
('sn_status','RETAINED','留库待定','Retained',9,'待处置中间态（非终态）'),
('sn_status','DISPOSED','已处置','Disposed',10,'处置类终态');

-- ② 判定结果
INSERT INTO enum_dict VALUES
('judgment_result','REPAIR','维修','Repair',1,NULL),
('judgment_result','REPLACE','换新','Replace',2,'归非原片还货(BUFFER)'),
('judgment_result','SCRAP','报废','Scrap',3,'原片报废+BUFFER还货'),
('judgment_result','TRANSFER_RC','转维修中心','Transfer to RC',4,'原片转中心+BUFFER还货'),
('judgment_result','CID_RETURN','CID原片返还','CID Return',5,'客户责，原片直接返还');

-- ③ 出库类型
INSERT INTO enum_dict VALUES
('outbound_type','SHIP_TO_CUSTOMER','发回客户','Ship to Customer',1,'含CID子类'),
('outbound_type','SHIP_TO_RC','发维修中心','Ship to RC',2,NULL),
('outbound_type','SCRAP_DISPOSAL','报废处置','Scrap Disposal',3,'需审批'),
('outbound_type','TRANSFER_WAREHOUSE','调拨转仓','Transfer Warehouse',4,'需移库单+清点确认');

-- ④ 差异类型
INSERT INTO enum_dict VALUES
('discrepancy_type','SN_NOT_IN_FORECAST','SN未在预报清单','SN not in forecast',1,'多收/来路不明'),
('discrepancy_type','PN_MISMATCH','料号不一致','PN mismatch',2,NULL),
('discrepancy_type','RMA_MISMATCH','RMA号不一致','RMA mismatch',3,NULL),
('discrepancy_type','QTY_OVER','多收','Qty over',4,NULL),
('discrepancy_type','QTY_SHORT','少收','Qty short',5,NULL);

-- ⑤ 异常类别
INSERT INTO enum_dict VALUES
('anomaly_category','FORECAST_ANOMALY','预报异常','Forecast anomaly',1,'永久保留'),
('anomaly_category','ARRIVAL_ANOMALY','到货异常','Arrival anomaly',2,'永久保留');

-- ⑥ 预报单状态
INSERT INTO enum_dict VALUES
('forecast_status','ENTERED','已录入','Entered',1,NULL),
('forecast_status','REVISED','已修正','Revised',2,'KITTING差异审批后');

-- ⑦ 差异审核状态（REJECTED 为 DC-04 必要扩展，见风险 R-001）
INSERT INTO enum_dict VALUES
('approval_status','PENDING','待审批','Pending',1,NULL),
('approval_status','APPROVED','已审批','Approved',2,NULL),
('approval_status','REJECTED','已驳回','Rejected',3,'DC-04 驳回解锁');

-- ⑧ 出库单状态
INSERT INTO enum_dict VALUES
('outbound_status','CREATED','已创建','Created',1,NULL),
('outbound_status','PENDING_APPROVAL','报废审批中','Pending approval',2,NULL),
('outbound_status','REJECTED','审批驳回','Rejected',3,'可重提交'),
('outbound_status','SHIPPED','已出库','Shipped',4,NULL);

-- ⑨ 照片业务类型（POD/BOX_LABEL/SHIPMENT 为 PH-03 扩展，见风险 R-002）
INSERT INTO enum_dict VALUES
('photo_biz_type','DISCREPANCY_EVIDENCE','差异佐证','Discrepancy evidence',1,NULL),
('photo_biz_type','CID_DAMAGE_EVIDENCE','CID损坏佐证','CID damage evidence',2,NULL),
('photo_biz_type','POD','物流签收凭证','POD',3,'PH-03'),
('photo_biz_type','BOX_LABEL','箱标','Box label',4,'PH-03'),
('photo_biz_type','SHIPMENT','发货图片','Shipment',5,'PH-03');

-- ⑩ RMA 状态
INSERT INTO enum_dict VALUES
('rma_status','OPEN','开单','Open',1,NULL),
('rma_status','RECEIVING','收货中','Receiving',2,NULL),
('rma_status','PROCESSING','处理中','Processing',3,'到齐判定后'),
('rma_status','PENDING_RETURN','待还货','Pending return',4,'全部SN判定完成'),
('rma_status','RETURNED','已还货','Returned',5,'100%还货'),
('rma_status','CLOSED','已关闭','Closed',6,'系统自动关闭');

-- ⑪ 还货方式
INSERT INTO enum_dict VALUES
('return_method','ORIGINAL_REPAIR','原片修复还货','Original repair',1,'对应REPAIR'),
('return_method','CID_RETURN','CID还货','CID return',2,'对应CID_RETURN'),
('return_method','BUFFER_RETURN','BUFFER还货','Buffer return',3,'对应REPLACE/SCRAP/TRANSFER_RC');

-- ⑫ 备件库存状态
INSERT INTO enum_dict VALUES
('buffer_stock_status','AVAILABLE','可用','Available',1,NULL),
('buffer_stock_status','RESERVED','预占','Reserved',2,'防并发超扣'),
('buffer_stock_status','OUT','已出库','Out',3,NULL);

-- ⑬ 原片处置
INSERT INTO enum_dict VALUES
('original_disposition','ORIGINAL_SHIPPED','原片发走','Original shipped',1,'原片修复/CID'),
('original_disposition','RETAINED','留库待定','Retained',2,'BUFFER还货'),
('original_disposition','SCRAPPED','报废处置','Scrapped',3,'SCRAP判定'),
('original_disposition','TRANSFERRED_RC','转维修中心','Transferred to RC',4,'TRANSFER_RC判定');

-- ⑭ RETAINED 后续处置
INSERT INTO enum_dict VALUES
('retained_disposition','SCRAPPED','报废处置','Scrapped',1,NULL),
('retained_disposition','RETURN_VENDOR','退供应商','Return to vendor',2,NULL),
('retained_disposition','REPAIR_TO_BUFFER','修好入buffer复用','Repair to buffer',3,NULL),
('retained_disposition','LONG_RETAIN','长期留库','Long retain',4,'不强制收尾');

-- 角色
INSERT INTO enum_dict VALUES
('role','warehouse_operator','仓库操作员','Warehouse Operator',1,NULL),
('role','project_staff','项目人员','Project Staff',2,NULL),
('role','inspector','质检员/R&D','Inspector / R&D',3,NULL),
('role','customer','客户/BOE US','Customer / BOE US',4,'一期不开放登录'),
('role','admin','系统管理员','Administrator',5,NULL);

-- 差异来源（DC-11）
INSERT INTO enum_dict VALUES
('discrepancy_source','RECEIVING','物流允收差异','Receiving',1,'仅放行不改预报'),
('discrepancy_source','KITTING','Kitting扫码差异','Kitting',2,'审批通过改预报REVISED');

-- ---------- 初始化租户与主数据样例 ----------
INSERT INTO tenant (tenant_id, tenant_name) VALUES ('BOE_US','BOE US')
  ON DUPLICATE KEY UPDATE tenant_name = VALUES(tenant_name);

INSERT INTO product_model (tenant_id, model_code, model_name) VALUES
 ('BOE_US','MD-55','55寸商用显示模组'),
 ('BOE_US','MD-65','65寸商用显示模组')
  ON DUPLICATE KEY UPDATE model_name = VALUES(model_name);

INSERT INTO part_number (tenant_id, pn_code, model_code, description) VALUES
 ('BOE_US','PN-A100','MD-55','55寸主板'),
 ('BOE_US','PN-B200','MD-65','65寸主板')
  ON DUPLICATE KEY UPDATE description = VALUES(description);
