package service

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/boe/rma/internal/errs"
	"github.com/boe/rma/internal/model"
)

// ImportForecastSN Excel 批量导入 SN 清单（FC-08 / R-015 模板列：SN, PN(可选), RMA(可选)）。
func (s *Service) ImportForecastSN(r io.Reader) ([]ForecastSN, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, errs.BadRequest("INVALID_EXCEL", "Excel 解析失败："+err.Error())
	}
	defer f.Close()
	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, errs.BadRequest("INVALID_EXCEL", "读取工作表失败")
	}
	if len(rows) == 0 {
		return nil, errs.BadRequest("EMPTY_EXCEL", "Excel 内容为空")
	}
	// 表头定位
	start := 0
	if len(rows[0]) > 0 && strings.EqualFold(strings.TrimSpace(rows[0][0]), "SN") {
		start = 1
	}
	out := make([]ForecastSN, 0, len(rows)-start)
	for i := start; i < len(rows); i++ {
		row := rows[i]
		if len(row) == 0 {
			continue
		}
		sn := normSN(row[0])
		if sn == "" {
			continue
		}
		pn := ""
		if len(row) > 1 {
			pn = trimAll(row[1])
		}
		out = append(out, ForecastSN{SN: sn, PNCode: pn})
	}
	if len(out) == 0 {
		return nil, errs.BadRequest("EMPTY_EXCEL", "未解析到有效 SN 行")
	}
	return out, nil
}

// ForecastSNTemplate 生成 SN 导入模板。
func (s *Service) ForecastSNTemplate() ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	sheet := "SN清单"
	idx, err := f.NewSheet(sheet)
	if err != nil {
		return nil, err
	}
	f.SetActiveSheet(idx)
	_ = f.DeleteSheet("Sheet1")
	headers := []string{"SN", "PN", "RMA_NO"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	// 示例行
	f.SetCellValue(sheet, "A2", "SN-SAMPLE-0001")
	f.SetCellValue(sheet, "B2", "PN-A100")
	f.SetCellValue(sheet, "C2", "RMA-9001")
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ExportForecast 预报导出（含 SN 与状态）。
func (s *Service) ExportForecast(forecastNo string) ([]byte, error) {
	var sns []model.SNItem
	if err := s.DB.Where("tenant_id = ? AND forecast_no = ?", s.Tenant, forecastNo).Order("sn").Find(&sns).Error; err != nil {
		return nil, err
	}
	f := excelize.NewFile()
	defer f.Close()
	sheet := "预报导出"
	idx, _ := f.NewSheet(sheet)
	f.SetActiveSheet(idx)
	_ = f.DeleteSheet("Sheet1")
	headers := []string{"SN", "PN", "RMA_NO", "状态", "判定结果", "还货方式", "入库时间", "出库时间"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}
	for r, it := range sns {
		row := r + 2
		in, outAt := "", ""
		if it.ReceivedAt != nil {
			in = it.ReceivedAt.Format("2006-01-02 15:04:05")
		}
		if it.OutboundAt != nil {
			outAt = it.OutboundAt.Format("2006-01-02 15:04:05")
		}
		vals := []string{it.SN, it.PNCode, it.RMANo, it.Status, it.JudgmentResult, it.ReturnMethod, in, outAt}
		for c, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(c+1, row)
			f.SetCellValue(sheet, cell, v)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MasterImportResult 主数据导入结果（MD-03）。
type MasterImportResult struct {
	ModelsInserted int      `json:"models_inserted"`
	PNsInserted    int      `json:"pns_inserted"`
	Errors         []string `json:"errors"`
}

// ImportMaster Excel 主数据导入（列：TYPE, CODE, NAME, MODEL）。
// TYPE=MODEL → 机型；TYPE=PN → 料号（MODEL 列必填）。异常行整表阻断（MD-03）。
func (s *Service) ImportMaster(r io.Reader) (*MasterImportResult, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, errs.BadRequest("INVALID_EXCEL", "Excel 解析失败")
	}
	defer f.Close()
	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		return nil, errs.BadRequest("INVALID_EXCEL", "读取工作表失败")
	}
	if len(rows) <= 1 {
		return nil, errs.BadRequest("EMPTY_EXCEL", "无有效数据行")
	}
	res := &MasterImportResult{}
	seen := map[string]bool{}
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 2 {
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行字段不完整", i+1))
			continue
		}
		typ := strings.ToUpper(strings.TrimSpace(row[0]))
		code := trimAll(row[1])
		name := ""
		modelCode := ""
		if len(row) > 2 {
			name = strings.TrimSpace(row[2])
		}
		if len(row) > 3 {
			modelCode = trimAll(row[3])
		}
		key := typ + "|" + code
		if seen[key] {
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行重复：%s", i+1, code))
			continue
		}
		seen[key] = true
		switch typ {
		case "MODEL":
			if code == "" || name == "" {
				res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行机型编码/名称必填", i+1))
				continue
			}
			var cnt int64
			s.DB.Model(&model.ProductModel{}).Where("tenant_id = ? AND model_code = ?", s.Tenant, code).Count(&cnt)
			if cnt > 0 {
				res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行机型已存在：%s", i+1, code))
				continue
			}
			if err := s.DB.Create(&model.ProductModel{
				TenantID: s.Tenant, ModelCode: code, ModelName: name, CreatedAt: s.now(),
			}).Error; err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行写入失败", i+1))
				continue
			}
			res.ModelsInserted++
		case "PN":
			if code == "" || modelCode == "" {
				res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行料号/机型必填", i+1))
				continue
			}
			var cnt int64
			s.DB.Model(&model.ProductModel{}).Where("tenant_id = ? AND model_code = ?", s.Tenant, modelCode).Count(&cnt)
			if cnt == 0 {
				res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行机型不存在：%s", i+1, modelCode))
				continue
			}
			s.DB.Model(&model.PartNumber{}).Where("tenant_id = ? AND pn_code = ?", s.Tenant, code).Count(&cnt)
			if cnt > 0 {
				res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行料号已存在：%s", i+1, code))
				continue
			}
			if err := s.DB.Create(&model.PartNumber{
				TenantID: s.Tenant, PNCode: code, ModelCode: modelCode, Description: name, CreatedAt: s.now(),
			}).Error; err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行写入失败", i+1))
				continue
			}
			res.PNsInserted++
		default:
			res.Errors = append(res.Errors, fmt.Sprintf("第 %d 行 TYPE 非法：%s", i+1, typ))
		}
	}
	if len(res.Errors) > 0 {
		return res, errs.BadRequest("MASTER_IMPORT_HAS_ERRORS",
			fmt.Sprintf("导入存在 %d 行异常，请修正后重新导入", len(res.Errors)))
	}
	return res, nil
}
