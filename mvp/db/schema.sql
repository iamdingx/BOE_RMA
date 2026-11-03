-- ============================================================
-- BOE US RMA 收发管理系统 — MySQL 8.0 建表 DDL
-- 基线：PRD V1.22（E1~E12 + 序列表 + 审批表 + 用户表）
-- 约定：所有业务表含 tenant_id（一期单租户，默认 'BOE_US'）
-- 字符集 utf8mb4 / InnoDB / 时间精确到秒
-- ============================================================
SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- ---------- E10 租户 ----------
CREATE TABLE IF NOT EXISTS tenant (
  tenant_id     VARCHAR(32)  NOT NULL,
  tenant_name   VARCHAR(128) NOT NULL,
  created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='租户（多租户预留，一期单租户）';

-- ---------- E10 产品机型 ----------
CREATE TABLE IF NOT EXISTS product_model (
  tenant_id   VARCHAR(32)  NOT NULL,
  model_code  VARCHAR(64)  NOT NULL,
  model_name  VARCHAR(128) NOT NULL,
  created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, model_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='产品机型';

-- ---------- E10 料号 ----------
CREATE TABLE IF NOT EXISTS part_number (
  tenant_id   VARCHAR(32)  NOT NULL,
  pn_code     VARCHAR(64)  NOT NULL,
  model_code  VARCHAR(64)  NOT NULL,
  description VARCHAR(255) NULL,
  created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, pn_code),
  KEY idx_pn_model (tenant_id, model_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='料号主数据（kitting 比对基准）';

-- ---------- 用户 / 角色（5 角色固定） ----------
CREATE TABLE IF NOT EXISTS sys_user (
  tenant_id    VARCHAR(32)  NOT NULL,
  user_id      VARCHAR(32)  NOT NULL,
  username     VARCHAR(64)  NOT NULL,
  password_hash VARCHAR(128) NOT NULL,
  display_name VARCHAR(64)  NOT NULL,
  role         VARCHAR(32)  NOT NULL COMMENT 'warehouse_operator/project_staff/inspector/customer/admin',
  enabled      TINYINT(1)   NOT NULL DEFAULT 1,
  created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, user_id),
  UNIQUE KEY uk_user_name (tenant_id, username),
  CONSTRAINT ck_user_role CHECK (role IN ('warehouse_operator','project_staff','inspector','customer','admin'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统用户（客户角色一期不开放登录）';

-- ---------- E1 预报单（= RMA 单 1:1） ----------
CREATE TABLE IF NOT EXISTS forecast_order (
  tenant_id          VARCHAR(32)  NOT NULL,
  forecast_no        VARCHAR(32)  NOT NULL COMMENT '预报单号（系统生成 F+YYMMDD+4位）',
  rma_no             VARCHAR(64)  NOT NULL COMMENT 'RMA 号',
  pn_code            VARCHAR(64)  NOT NULL COMMENT '主 PN（多 PN 时存首个，明细见 forecast_sn_item）',
  model_code         VARCHAR(64)  NULL,
  forecast_qty       INT          NOT NULL DEFAULT 0 COMMENT '预报数量（= SN 清单条数）',
  customer           VARCHAR(128) NOT NULL,
  status             VARCHAR(16)  NOT NULL DEFAULT 'ENTERED' COMMENT 'ENTERED/REVISED',
  rma_status         VARCHAR(24)  NOT NULL DEFAULT 'OPEN' COMMENT 'OPEN/RECEIVING/PROCESSING/PENDING_RETURN/RETURNED/CLOSED',
  created_by         VARCHAR(64)  NOT NULL,
  created_at         DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  full_received_at   DATETIME     NULL COMMENT 'TAT 起算点（到齐判定成立时点）',
  closed_at          DATETIME     NULL COMMENT 'TAT 终点（=RMA 关闭同一时点）',
  tat_days           INT          NULL COMMENT 'ceil((closed_at-full_received_at)/24h)',
  tat_warning        TINYINT(1)   NOT NULL DEFAULT 0 COMMENT '第8天预警',
  tat_overdue        TINYINT(1)   NOT NULL DEFAULT 0 COMMENT '>10天标红',
  label_qty_total    INT          NULL COMMENT '物流允收：外箱标识数量合计（V1.22）',
  PRIMARY KEY (tenant_id, forecast_no),
  UNIQUE KEY uk_rma (tenant_id, rma_no),
  KEY idx_fo_status (tenant_id, rma_status),
  KEY idx_fo_customer (tenant_id, customer),
  CONSTRAINT ck_fo_status CHECK (status IN ('ENTERED','REVISED')),
  CONSTRAINT ck_fo_rma_status CHECK (rma_status IN ('OPEN','RECEIVING','PROCESSING','PENDING_RETURN','RETURNED','CLOSED'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='E1 预报单 / RMA 单';

-- ---------- E2 预报 SN 明细 ----------
CREATE TABLE IF NOT EXISTS forecast_sn_item (
  tenant_id   VARCHAR(32) NOT NULL,
  forecast_no VARCHAR(32) NOT NULL,
  sn          VARCHAR(128) NOT NULL,
  rma_no      VARCHAR(64) NOT NULL,
  pn_code     VARCHAR(64) NOT NULL,
  PRIMARY KEY (tenant_id, forecast_no, sn),
  KEY idx_fsi_sn (tenant_id, sn),
  CONSTRAINT fk_fsi_fo FOREIGN KEY (tenant_id, forecast_no) REFERENCES forecast_order (tenant_id, forecast_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='E2 预报 SN 明细';

-- ---------- E3 允收批次（POD 为整批） ----------
CREATE TABLE IF NOT EXISTS receiving_batch (
  tenant_id       VARCHAR(32)  NOT NULL,
  batch_no        VARCHAR(32)  NOT NULL COMMENT 'RCV+YYMMDD+3位',
  forecast_no     VARCHAR(32)  NOT NULL,
  pod_photo_ids   JSON         NULL COMMENT 'POD 照片 ID 数组（支持多张）',
  label_qty_total INT          NOT NULL DEFAULT 0 COMMENT '外箱标识数量合计（V1.22）',
  qty_diff        INT          NOT NULL DEFAULT 0 COMMENT 'label_qty_total - forecast_qty',
  status          VARCHAR(16)  NOT NULL DEFAULT 'RECEIVED' COMMENT 'RECEIVED/LOCKED',
  received_by     VARCHAR(64)  NOT NULL,
  received_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  is_late_receive TINYINT(1)   NOT NULL DEFAULT 0 COMMENT 'CLOSED_SHORT 迟到货重收（DC-10）',
  PRIMARY KEY (tenant_id, batch_no),
  KEY idx_rb_fo (tenant_id, forecast_no),
  CONSTRAINT fk_rb_fo FOREIGN KEY (tenant_id, forecast_no) REFERENCES forecast_order (tenant_id, forecast_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='E3 物流允收批次（只点箱数/外箱标识数量，不开箱）';

-- ---------- E3-B 收货箱（kitting 开箱生成） ----------
CREATE TABLE IF NOT EXISTS box_info (
  tenant_id       VARCHAR(32)  NOT NULL,
  box_no          VARCHAR(32)  NOT NULL COMMENT 'BOE+YYMMDD+3位流水（独立序列）',
  batch_no        VARCHAR(32)  NOT NULL,
  forecast_no     VARCHAR(32)  NOT NULL,
  sn_list         JSON         NULL COMMENT '箱内 SN 明细（冗余快照）',
  status          VARCHAR(16)  NOT NULL DEFAULT 'OPEN' COMMENT 'OPEN/SEALED',
  label_print_log JSON         NULL COMMENT '打印时间/操作人；标签=箱号+SN明细',
  sealed_by       VARCHAR(64)  NULL,
  sealed_at       DATETIME     NULL,
  created_by      VARCHAR(64)  NOT NULL,
  created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, box_no),
  KEY idx_box_batch (tenant_id, batch_no),
  CONSTRAINT fk_box_rb FOREIGN KEY (tenant_id, batch_no) REFERENCES receiving_batch (tenant_id, batch_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='E3-B 收货箱';

CREATE TABLE IF NOT EXISTS box_sn_item (
  tenant_id VARCHAR(32)  NOT NULL,
  box_no    VARCHAR(32)  NOT NULL,
  sn        VARCHAR(128) NOT NULL,
  PRIMARY KEY (tenant_id, box_no, sn),
  KEY idx_bsi_sn (tenant_id, sn),
  CONSTRAINT fk_bsi_box FOREIGN KEY (tenant_id, box_no) REFERENCES box_info (tenant_id, box_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='收货箱内 SN 明细';

-- ---------- E4 SN 单品记录（核心） ----------
CREATE TABLE IF NOT EXISTS sn_item (
  tenant_id            VARCHAR(32)  NOT NULL,
  sn                   VARCHAR(128) NOT NULL,
  status               VARCHAR(24)  NOT NULL COMMENT 'FORECASTED/RECEIVED/DISCREPANCY/INSPECTING/JUDGED/OUTBOUND/COMPLETED/CLOSED_SHORT/RETAINED/DISPOSED',
  rma_no               VARCHAR(64)  NULL,
  pn_code              VARCHAR(64)  NULL,
  forecast_no          VARCHAR(32)  NULL COMMENT '为空=多收未预报实物（SN-05）',
  batch_no             VARCHAR(32)  NULL,
  box_no               VARCHAR(32)  NULL,
  judgment_result      VARCHAR(16)  NULL COMMENT 'REPAIR/REPLACE/SCRAP/TRANSFER_RC/CID_RETURN',
  return_method        VARCHAR(24)  NULL COMMENT 'ORIGINAL_REPAIR/CID_RETURN/BUFFER_RETURN（三段式自动带出）',
  outbound_no          VARCHAR(32)  NULL,
  return_count         INT          NOT NULL DEFAULT 0 COMMENT '返修次数',
  secondary_return_flag TINYINT(1)  NOT NULL DEFAULT 0,
  cid_flag             TINYINT(1)   NOT NULL DEFAULT 0,
  owner                VARCHAR(64)  NULL,
  locked               TINYINT(1)   NOT NULL DEFAULT 0 COMMENT 'kitting 提交锁定（KT-04）',
  received_at          DATETIME     NULL,
  inspected_at         DATETIME     NULL,
  judged_at            DATETIME     NULL,
  outbound_at          DATETIME     NULL,
  completed_at         DATETIME     NULL,
  updated_at           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, sn),
  KEY idx_sn_status (tenant_id, status),
  KEY idx_sn_rma (tenant_id, rma_no),
  KEY idx_sn_forecast (tenant_id, forecast_no),
  KEY idx_sn_box (tenant_id, box_no),
  CONSTRAINT ck_sn_status CHECK (status IN ('FORECASTED','RECEIVED','DISCREPANCY','INSPECTING','JUDGED','OUTBOUND','COMPLETED','CLOSED_SHORT','RETAINED','DISPOSED')),
  CONSTRAINT ck_sn_judgment CHECK (judgment_result IS NULL OR judgment_result IN ('REPAIR','REPLACE','SCRAP','TRANSFER_RC','CID_RETURN')),
  CONSTRAINT ck_sn_method CHECK (return_method IS NULL OR return_method IN ('ORIGINAL_REPAIR','CID_RETURN','BUFFER_RETURN'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='E4 SN 单品记录（tenant+sn 全局唯一）';

-- ---------- E4-B SN 历史轮次 ----------
CREATE TABLE IF NOT EXISTS sn_item_round (
  tenant_id       VARCHAR(32)  NOT NULL,
  round_id        VARCHAR(40)  NOT NULL COMMENT 'SN-R{round_no}',
  sn              VARCHAR(128) NOT NULL,
  round_no        INT          NOT NULL COMMENT '第几轮（1=首次）',
  rma_no          VARCHAR(64)  NULL,
  pn_code         VARCHAR(64)  NULL,
  forecast_no     VARCHAR(32)  NULL,
  from_status     VARCHAR(24)  NOT NULL,
  to_status       VARCHAR(24)  NOT NULL,
  started_at      DATETIME     NULL,
  ended_at        DATETIME     NULL,
  received_at     DATETIME     NULL,
  judged_at       DATETIME     NULL,
  outbound_at     DATETIME     NULL,
  judgment_result VARCHAR(16)  NULL,
  outbound_type   VARCHAR(24)  NULL,
  PRIMARY KEY (tenant_id, round_id),
  UNIQUE KEY uk_sn_round (tenant_id, sn, round_no),
  CONSTRAINT fk_round_sn FOREIGN KEY (tenant_id, sn) REFERENCES sn_item (tenant_id, sn)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='E4-B SN 历史轮次归档';

-- ---------- E5 差异记录（双异常，永久不可删） ----------
CREATE TABLE IF NOT EXISTS discrepancy_record (
  tenant_id        VARCHAR(32)  NOT NULL,
  record_no        VARCHAR(40)  NOT NULL,
  category         VARCHAR(24)  NOT NULL COMMENT 'FORECAST_ANOMALY/ARRIVAL_ANOMALY',
  source           VARCHAR(16)  NOT NULL COMMENT 'RECEIVING/KITTING（DC-11）',
  discrepancy_type VARCHAR(24)  NOT NULL COMMENT 'SN_NOT_IN_FORECAST/PN_MISMATCH/RMA_MISMATCH/QTY_OVER/QTY_SHORT',
  forecast_no      VARCHAR(32)  NULL,
  batch_no         VARCHAR(32)  NULL,
  box_no           VARCHAR(32)  NULL,
  sn               VARCHAR(128) NULL COMMENT '数量类差异可空',
  pn_code          VARCHAR(64)  NULL,
  rma_no           VARCHAR(64)  NULL,
  missing_sn_list  JSON         NULL COMMENT '少收未到货 SN 清单（DC-09）',
  qty_diff         INT          NOT NULL DEFAULT 0,
  photo_ids        JSON         NULL,
  approval_status  VARCHAR(16)  NOT NULL DEFAULT 'PENDING' COMMENT 'PENDING/APPROVED/REJECTED（R-001 扩展）',
  approver         VARCHAR(64)  NULL,
  approved_at      DATETIME     NULL,
  approval_note    TEXT         NULL,
  reject_reason    TEXT         NULL,
  discovered_by    VARCHAR(64)  NOT NULL,
  discovered_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  locked           TINYINT(1)   NOT NULL DEFAULT 1 COMMENT '驳回时置0解锁重扫（RK-11）',
  PRIMARY KEY (tenant_id, record_no),
  KEY idx_dr_status (tenant_id, approval_status),
  KEY idx_dr_forecast (tenant_id, forecast_no),
  KEY idx_dr_sn (tenant_id, sn),
  CONSTRAINT ck_dr_cat CHECK (category IN ('FORECAST_ANOMALY','ARRIVAL_ANOMALY')),
  CONSTRAINT ck_dr_src CHECK (source IN ('RECEIVING','KITTING')),
  CONSTRAINT ck_dr_type CHECK (discrepancy_type IN ('SN_NOT_IN_FORECAST','PN_MISMATCH','RMA_MISMATCH','QTY_OVER','QTY_SHORT')),
  CONSTRAINT ck_dr_appr CHECK (approval_status IN ('PENDING','APPROVED','REJECTED'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='E5 差异记录（双异常永久保留）';

-- ---------- E6 质检判定 ----------
CREATE TABLE IF NOT EXISTS inspection_disposition (
  tenant_id            VARCHAR(32)  NOT NULL,
  record_id            VARCHAR(40)  NOT NULL,
  sn                   VARCHAR(128) NOT NULL,
  judgment_result      VARCHAR(16)  NOT NULL,
  cid_flag             TINYINT(1)   NOT NULL DEFAULT 0,
  damage_photo_ids     JSON         NULL COMMENT 'CID 必填 ≥1',
  customer_confirmation TEXT        NULL COMMENT 'CID 时项目人员确认（必填）；客户佐证选填',
  inspector            VARCHAR(64)  NOT NULL,
  inspection_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, record_id),
  KEY idx_id_sn (tenant_id, sn),
  CONSTRAINT ck_id_judgment CHECK (judgment_result IN ('REPAIR','REPLACE','SCRAP','TRANSFER_RC','CID_RETURN'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='E6 质检判定记录';

-- ---------- E7 出库单 ----------
CREATE TABLE IF NOT EXISTS outbound_order (
  tenant_id        VARCHAR(32)  NOT NULL,
  outbound_no      VARCHAR(32)  NOT NULL COMMENT 'OB- 前缀单号',
  outbound_type    VARCHAR(24)  NOT NULL COMMENT 'SHIP_TO_CUSTOMER/SHIP_TO_RC/SCRAP_DISPOSAL/TRANSFER_WAREHOUSE',
  status           VARCHAR(20)  NOT NULL DEFAULT 'CREATED' COMMENT 'CREATED/PENDING_APPROVAL/REJECTED/SHIPPED',
  rma_no           VARCHAR(64)  NULL,
  logistics_no     VARCHAR(64)  NULL,
  carrier          VARCHAR(64)  NULL,
  shipped_at       DATETIME     NULL,
  scrap_approval   JSON         NULL COMMENT '审批人/时间/意见（项目人员）',
  transfer_no      VARCHAR(64)  NULL,
  count_confirmed_by VARCHAR(64) NULL,
  count_confirmed_at DATETIME   NULL,
  cid_flag         TINYINT(1)   NOT NULL DEFAULT 0,
  operator         VARCHAR(64)  NOT NULL,
  outbound_at      DATETIME     NULL,
  created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, outbound_no),
  KEY idx_ob_status (tenant_id, status),
  KEY idx_ob_type (tenant_id, outbound_type),
  CONSTRAINT ck_ob_type CHECK (outbound_type IN ('SHIP_TO_CUSTOMER','SHIP_TO_RC','SCRAP_DISPOSAL','TRANSFER_WAREHOUSE')),
  CONSTRAINT ck_ob_status CHECK (status IN ('CREATED','PENDING_APPROVAL','REJECTED','SHIPPED'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='E7 出库单';

CREATE TABLE IF NOT EXISTS outbound_sn_item (
  tenant_id   VARCHAR(32)  NOT NULL,
  outbound_no VARCHAR(32)  NOT NULL,
  sn          VARCHAR(128) NOT NULL,
  PRIMARY KEY (tenant_id, outbound_no, sn),
  KEY idx_osi_sn (tenant_id, sn),
  CONSTRAINT fk_osi_ob FOREIGN KEY (tenant_id, outbound_no) REFERENCES outbound_order (tenant_id, outbound_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='出库明细 SN';

-- ---------- E7-B 发货箱 ----------
CREATE TABLE IF NOT EXISTS shipment_box (
  tenant_id        VARCHAR(32)  NOT NULL,
  shipment_box_no  VARCHAR(32)  NOT NULL COMMENT 'OB+YYMMDD+3位流水（独立序列，禁止与收货箱重号）',
  rma_nos          JSON         NULL COMMENT '该箱还货 RMA（可混装多个）',
  outbound_no      VARCHAR(32)  NOT NULL,
  sn_list          JSON         NULL,
  weight           DECIMAL(10,3) NULL COMMENT 'kg',
  volume           DECIMAL(10,4) NULL COMMENT 'm³',
  carrier          VARCHAR(64)  NULL,
  tracking_no      VARCHAR(64)  NULL,
  label_print_log  JSON         NULL,
  packed_by        VARCHAR(64)  NOT NULL,
  packed_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  sealed           TINYINT(1)   NOT NULL DEFAULT 0,
  shipped_at       DATETIME     NULL,
  PRIMARY KEY (tenant_id, shipment_box_no),
  KEY idx_sb_outbound (tenant_id, outbound_no),
  CONSTRAINT fk_sb_ob FOREIGN KEY (tenant_id, outbound_no) REFERENCES outbound_order (tenant_id, outbound_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='E7-B 发货箱（逐箱打包，整单发运）';

CREATE TABLE IF NOT EXISTS shipment_box_sn (
  tenant_id        VARCHAR(32)  NOT NULL,
  shipment_box_no  VARCHAR(32)  NOT NULL,
  sn               VARCHAR(128) NOT NULL,
  PRIMARY KEY (tenant_id, shipment_box_no, sn),
  CONSTRAINT fk_sbs_sb FOREIGN KEY (tenant_id, shipment_box_no) REFERENCES shipment_box (tenant_id, shipment_box_no)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='发货箱内 SN';

-- ---------- E8 照片证据（≥2年留存） ----------
CREATE TABLE IF NOT EXISTS photo_evidence (
  tenant_id       VARCHAR(32)  NOT NULL,
  photo_id        VARCHAR(40)  NOT NULL,
  biz_type        VARCHAR(32)  NOT NULL COMMENT 'DISCREPANCY_EVIDENCE/CID_DAMAGE_EVIDENCE/POD/BOX_LABEL/SHIPMENT（R-002 扩展）',
  biz_ref_no      VARCHAR(64)  NOT NULL,
  sn              VARCHAR(128) NULL,
  pn_code         VARCHAR(64)  NULL,
  object_key      VARCHAR(255) NOT NULL,
  original_url    VARCHAR(512) NOT NULL,
  thumbnail_url   VARCHAR(512) NULL,
  file_size       BIGINT       NOT NULL COMMENT '≤10MB',
  content_type    VARCHAR(64)  NULL,
  uploaded_by     VARCHAR(64)  NOT NULL,
  uploaded_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  retention_until DATE         NOT NULL COMMENT 'uploaded_at + 2年',
  PRIMARY KEY (tenant_id, photo_id),
  KEY idx_pe_biz (tenant_id, biz_type, biz_ref_no),
  CONSTRAINT ck_pe_type CHECK (biz_type IN ('DISCREPANCY_EVIDENCE','CID_DAMAGE_EVIDENCE','POD','BOX_LABEL','SHIPMENT'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='E8 照片附件';

-- ---------- E9 审计日志（只增不删） ----------
CREATE TABLE IF NOT EXISTS audit_log (
  log_id     BIGINT       NOT NULL AUTO_INCREMENT,
  tenant_id  VARCHAR(32)  NOT NULL,
  biz_type   VARCHAR(32)  NOT NULL,
  biz_no     VARCHAR(64)  NOT NULL,
  action     VARCHAR(40)  NOT NULL COMMENT 'CONTROLLED_CORRECTION/RETURN_METHOD_CHANGE/RETAINED_DISPOSE/BUFFER_SUBSTITUTE_APPROVAL/SCRAP_APPROVAL/RMA_CLOSE/DISCREPANCY_APPROVE/DISCREPANCY_REJECT/INSPECTION_JUDGE/OUTBOUND_SHIP/BOX_SEAL/PHOTO_UPLOAD/LOGIN',
  field      VARCHAR(64)  NULL,
  old_value  TEXT         NULL,
  new_value  TEXT         NULL,
  reason     TEXT         NULL,
  operator   VARCHAR(64)  NOT NULL,
  approved_by VARCHAR(64) NULL,
  created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (log_id),
  KEY idx_al_tenant (tenant_id),
  KEY idx_al_biz (tenant_id, biz_type, biz_no),
  KEY idx_al_action (tenant_id, action),
  KEY idx_al_time (tenant_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='E9 审计日志';

-- ---------- 箱号独立序列（BOE / OB 两套） ----------
CREATE TABLE IF NOT EXISTS box_sequence (
  tenant_id  VARCHAR(32) NOT NULL,
  seq_type   VARCHAR(16) NOT NULL COMMENT 'RECEIVE(BOE) / SHIP(OB)',
  date_key   CHAR(6)     NOT NULL COMMENT 'YYMMDD',
  current_no INT         NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, seq_type, date_key),
  CONSTRAINT ck_seq_type CHECK (seq_type IN ('RECEIVE','SHIP'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='箱号流水（两套独立序列，单日≤999）';

-- ---------- 通用业务单号序列 ----------
CREATE TABLE IF NOT EXISTS biz_sequence (
  tenant_id  VARCHAR(32) NOT NULL,
  seq_type   VARCHAR(24) NOT NULL COMMENT 'FORECAST/BATCH/OUTBOUND/RETURN/DISCREPANCY/BSA/INBOUND',
  date_key   CHAR(6)     NOT NULL COMMENT 'YYMMDD',
  current_no INT         NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, seq_type, date_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='通用业务单号流水';

-- ---------- E11 缓冲备件 ----------
CREATE TABLE IF NOT EXISTS buffer_stock (
  tenant_id        VARCHAR(32)  NOT NULL,
  sn               VARCHAR(128) NOT NULL,
  pn_code          VARCHAR(64)  NOT NULL,
  source           VARCHAR(20)  NOT NULL COMMENT 'NEW_PURCHASE/REPAIRED_GOOD',
  stock_status     VARCHAR(16)  NOT NULL DEFAULT 'AVAILABLE' COMMENT 'AVAILABLE/RESERVED/OUT',
  inbound_order_no VARCHAR(32)  NULL,
  inbound_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  inbound_by       VARCHAR(64)  NOT NULL,
  return_record_no VARCHAR(40)  NULL COMMENT 'BUFFER 还货出库后关联',
  reserved_by      VARCHAR(40)  NULL COMMENT '预占单号（防并发）',
  reserved_at      DATETIME     NULL,
  PRIMARY KEY (tenant_id, sn),
  KEY idx_bs_pn (tenant_id, pn_code, stock_status),
  CONSTRAINT ck_bs_source CHECK (source IN ('NEW_PURCHASE','REPAIRED_GOOD')),
  CONSTRAINT ck_bs_status CHECK (stock_status IN ('AVAILABLE','RESERVED','OUT','SCRAPPED'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='E11 缓冲备件库存（独立表，不进 sn_item）';

-- ---------- BUFFER 备件入库单（项目人员预报，仓库执行） ----------
CREATE TABLE IF NOT EXISTS buffer_inbound_order (
  tenant_id   VARCHAR(32) NOT NULL,
  order_no    VARCHAR(32) NOT NULL,
  pn_code     VARCHAR(64) NOT NULL,
  source      VARCHAR(20) NOT NULL,
  qty_expected INT        NOT NULL,
  status      VARCHAR(16) NOT NULL DEFAULT 'CREATED' COMMENT 'CREATED/COMPLETED',
  created_by  VARCHAR(64) NOT NULL,
  created_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, order_no),
  CONSTRAINT ck_bio_source CHECK (source IN ('NEW_PURCHASE','REPAIRED_GOOD'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='BUFFER 备件入库预报';

-- ---------- 跨 PN 替代还货审批 ----------
CREATE TABLE IF NOT EXISTS buffer_substitute_approval (
  tenant_id      VARCHAR(32)  NOT NULL,
  approval_no    VARCHAR(40)  NOT NULL,
  original_sn    VARCHAR(128) NOT NULL,
  original_pn    VARCHAR(64)  NOT NULL,
  substitute_sn  VARCHAR(128) NOT NULL,
  substitute_pn  VARCHAR(64)  NOT NULL,
  status         VARCHAR(16)  NOT NULL DEFAULT 'PENDING' COMMENT 'PENDING/APPROVED/REJECTED',
  approver       VARCHAR(64)  NULL,
  approved_at    DATETIME     NULL,
  note           TEXT         NULL,
  requested_by   VARCHAR(64)  NOT NULL,
  created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, approval_no),
  KEY idx_bsa_status (tenant_id, status),
  CONSTRAINT ck_bsa_status CHECK (status IN ('PENDING','APPROVED','REJECTED'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='BS-08 跨 PN 替代还货独立审批';

-- ---------- E12 还货记录 ----------
CREATE TABLE IF NOT EXISTS return_record (
  tenant_id             VARCHAR(32)  NOT NULL,
  return_no             VARCHAR(40)  NOT NULL,
  rma_no                VARCHAR(64)  NOT NULL,
  forecast_no           VARCHAR(32)  NOT NULL,
  original_sn           VARCHAR(128) NOT NULL COMMENT '退货原片',
  return_method         VARCHAR(24)  NOT NULL,
  returned_sn           VARCHAR(128) NOT NULL COMMENT '实际发客户 SN',
  outbound_no           VARCHAR(32)  NULL,
  shipment_box_no       VARCHAR(32)  NULL,
  original_disposition  VARCHAR(24)  NOT NULL COMMENT 'ORIGINAL_SHIPPED/RETAINED/SCRAPPED/TRANSFERRED_RC',
  retained_disposition  VARCHAR(24)  NULL COMMENT 'SCRAPPED/RETURN_VENDOR/REPAIR_TO_BUFFER/LONG_RETAIN',
  status                VARCHAR(16)  NOT NULL DEFAULT 'PENDING' COMMENT 'PENDING/RESERVED/SHIPPED',
  returned_at           DATETIME     NULL,
  operator              VARCHAR(64)  NULL,
  created_at            DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (tenant_id, return_no),
  KEY idx_rr_rma (tenant_id, rma_no),
  KEY idx_rr_orig (tenant_id, original_sn),
  UNIQUE KEY uk_rr_orig_round (tenant_id, forecast_no, original_sn),
  CONSTRAINT ck_rr_method CHECK (return_method IN ('ORIGINAL_REPAIR','CID_RETURN','BUFFER_RETURN')),
  CONSTRAINT ck_rr_disp CHECK (original_disposition IN ('ORIGINAL_SHIPPED','RETAINED','SCRAPPED','TRANSFERRED_RC')),
  CONSTRAINT ck_rr_retained CHECK (retained_disposition IS NULL OR retained_disposition IN ('SCRAPPED','RETURN_VENDOR','REPAIR_TO_BUFFER','LONG_RETAIN'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='E12 还货记录';

-- ---------- 防删除触发器（DC-05 双异常 + E9 审计 + E8 照片） ----------
DROP TRIGGER IF EXISTS trg_dr_no_delete;
CREATE TRIGGER trg_dr_no_delete BEFORE DELETE ON discrepancy_record
FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'discrepancy_record is append-only (DC-05)';

DROP TRIGGER IF EXISTS trg_al_no_delete;
CREATE TRIGGER trg_al_no_delete BEFORE DELETE ON audit_log
FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'audit_log is append-only (M-14)';

DROP TRIGGER IF EXISTS trg_pe_no_delete;
CREATE TRIGGER trg_pe_no_delete BEFORE DELETE ON photo_evidence
FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'photo_evidence retention >= 2y (PH-03)';

DROP TRIGGER IF EXISTS trg_round_no_delete;
CREATE TRIGGER trg_round_no_delete BEFORE DELETE ON sn_item_round
FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'sn_item_round is history archive';

SET FOREIGN_KEY_CHECKS = 1;
