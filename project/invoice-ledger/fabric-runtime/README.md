# 本地 Fabric 运行环境

本目录使项目不再依赖课程 `lab7` 目录。其标准结构为：

```text
fabric-runtime/
├── bin/           peer、cryptogen、configtxgen、osnadmin
├── config/        Fabric 配置文件
└── test-network/  Fabric Test Network
```

项目脚本默认使用 `fabric-runtime/test-network`。因此在 Ubuntu 虚拟机或 WSL 中，只需完整复制项目目录并保证 Docker、Docker Compose 和 Go 可用，即可执行：

```bash
./scripts/start-network.sh
./scripts/deploy-chaincode.sh
./scripts/start-app.sh
```

`bin/`、运行过程中生成的证书、通道材料和账本状态均不会提交到 Git，因为其中包含大体积二进制和本地测试密钥。通过 Git 克隆项目后，请从原开发环境复制整个 `fabric-runtime/` 目录，或按 Fabric 官方安装方式准备同版本的二进制与配置文件。

如需使用其他 Fabric 网络，可在运行脚本前覆盖默认路径：

```bash
export FABRIC_TEST_NETWORK=/绝对路径/fabric-samples/test-network
```
