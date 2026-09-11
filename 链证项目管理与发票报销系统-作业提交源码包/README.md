# 链证项目管理与发票报销系统

本仓库是“区块链技术应用实践”课程项目的源码。系统使用 Hyperledger Fabric 保存项目、发票、报销、审批与支付等关键业务记录，并提供浏览器端的组织管理、OCR 发票识别、项目资金池和数据统计功能。

## 快速运行

首次部署空白网络：

```bash
./scripts/start-network.sh check
./scripts/start-network.sh init
./scripts/deploy-chaincode.sh
./scripts/start-app.sh
```

已部署过的网络在电脑重启后恢复：

```bash
./scripts/start-network.sh resume
./scripts/start-app.sh
```

浏览器访问 <http://localhost:8080>。

## 课程提交包

如果目录中存在 `ledger-state/`，说明这是带预置演示数据的作业提交包。请优先阅读 [作业提交运行说明.md](作业提交运行说明.md)，先恢复账本状态，再启动网络和应用。

其他说明见：[系统使用手册.md](系统使用手册.md) 和 [组员部署与快速上手手册.md](组员部署与快速上手手册.md)。
