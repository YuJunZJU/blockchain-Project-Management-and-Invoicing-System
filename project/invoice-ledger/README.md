# 链证发票｜Fabric 发票存证、流转与核验系统

本项目是“区块链技术应用实践”课程大作业的源代码。它围绕电子发票的**存证、流转、核验**构建了可在浏览器访问的 B/S 应用，区块链层使用 Hyperledger Fabric。

## 功能

- 发票存证：录入发票要素，后端生成 SHA-256 内容指纹，链码将发票及指纹写入 Fabric 账本。
- 发票流转：当前持有人可将发票流转给下一主体；每次变更均生成独立、不可覆盖的流转凭据。
- 发票核验：用发票编号和内容指纹核验链上记录，并可查看完整流转链与 Fabric 历史记录。
- 链上业务用户：注册姓名、组织与岗位；发票接收人与下一持有人必须是链上已注册的流转员。
- 项目与报销：项目申请、审核意见、资金池、报销审核与支付状态均由固定状态机约束并上链。
- 可视化：提供概览统计、发票清单、详情抽屉和核验结果展示。

## 目录

```text
invoice-ledger/
├── chaincode/             Fabric 智能合约（Go）
├── backend/               Gin REST API 与 Fabric Gateway SDK（Go）
├── web/                   浏览器前端（HTML/CSS/JavaScript）
├── scripts/               启动网络、部署链码、启动应用脚本
├── 实现步骤.md             开发、部署与验收步骤
├── 区块链与系统架构学习指南.md  区块链与项目架构入门资料
├── 权限与升级说明.md        作废、多组织身份和角色权限操作说明
├── 项目与报销模块说明.md      项目申请、资金池与报销演示说明
├── 项目介绍.md             可直接扩展为提交材料的项目说明
├── 答辩材料大纲.md         PPT 与录屏脚本建议
└── 团队分工模板.md         成员贡献登记模板
```

## 快速启动

先按 [实现步骤.md](实现步骤.md) 完成 Go、Docker 和 Fabric 环境准备。随后，在本目录执行：

```bash
./scripts/start-network.sh
./scripts/deploy-chaincode.sh
./scripts/start-app.sh
```

浏览器打开 `http://localhost:8080`。默认复用课程 `lab7` 目录中的 Fabric Test Network；若其不在默认位置，执行脚本前设置 `FABRIC_TEST_NETWORK` 为 `test-network` 的绝对路径。

如果你的网络已经部署过 `invoice` 链码 1.0 / Sequence 1，请按 [权限与升级说明.md](权限与升级说明.md) 使用 `upgrade-chaincode.sh` 升级到新版链码，而不是重复执行普通部署脚本。

## 核心接口

| 方法 | 地址 | 用途 |
| --- | --- | --- |
| POST | `/api/invoices` | 创建并存证发票 |
| GET | `/api/invoices` | 查询全部发票 |
| GET | `/api/invoices/:id` | 查询单张发票 |
| POST | `/api/invoices/:id/transfers` | 发票流转 |
| GET | `/api/invoices/:id/flows` | 查询流转链 |
| GET | `/api/invoices/:id/history` | 查询 Fabric 历史 |
| POST | `/api/invoices/:id/verify` | 校验内容指纹 |

## 内容指纹规则

后端以如下稳定字段顺序拼接并计算 SHA-256：

```text
发票号码|开票日期|销售方|购买方|金额（分）|税额（分）|价税合计（分）|币种
```

金额统一以“分”写入链上，避免浮点数精度问题。演示中可将新建发票返回的 64 位 `dataHash` 复制到“发票核验”区域，展示真伪核验；任意修改一位哈希都会得到“不匹配”的结果。
