一些不是很懂的概念

1.什么是`Fabric Gateway`

```
Fabric 区块链网络
  ├── Peer：执行链码、保存账本
  ├── Orderer：排序交易、生成区块
  ├── CA：提供测试身份/证书
  └── invoice 链码：执行发票业务规则
```

这几个分别具体的内容

3.Docker相关

4.`mychannel`是什么

5.几个脚本运行的顺序是什么

6.Fabric是有什么固定的接口吗

7.发票流转是什么啊

8.用户的概念

9.具体讲讲`后端用 Fabric 证书签名交易`z这一步

10.世界状态和区块历史相关，怎么看

11.我现在进入浏览器这个端口，填写信息，我算什么角色啊

12.什么叫持有人啊

13.什么是`gRPC 连接`

14.不懂`peer0.org1.example.com` 和 `peer0.org2.example.com`什么区别啊

15.关于Orderer我想问的是，他只是负责对交易顺序进行排序是吗，并没有决定谁负责持有下一个区块

16.不是很懂peer和用户之间的关系是什么

17.现在这个系统中channel只有一个吗

---

# 解答

> 先记住一个重点：当前是课程演示版。网页没有真实登录功能，所有链上写入交易都由后端统一使用 Org1 的测试证书签名；你在表单中填写的销售方、购买方、操作人、持有人，是我们设计的**业务字段**。

## 1. 什么是 `Fabric Gateway`？

`Fabric Gateway` 是普通后端连接 Fabric 区块链网络的客户端入口。浏览器不直接连接 Fabric，因为浏览器不应该保存企业私钥，也不适合直接处理 Peer 的 gRPC 连接。

本项目的 Gateway 代码在：`backend/fabric/gateway.go`。它会：

1. 读取 Org1 测试用户的证书；
2. 读取该用户的私钥；
3. 连接 `peer0.org1.example.com:7051`；
4. 获得 `mychannel` 上 `invoice` 链码的调用对象。

后端通过它调用链码：

```go
// 写入交易：会经过背书、排序并写入区块
fabric.Contract.SubmitTransaction("CreateInvoice", ...)

// 查询交易：只读，不产生新区块
fabric.Contract.EvaluateTransaction("GetAllInvoices")
```

可以类比为：网页调用普通后端用 HTTP；普通后端调用 Fabric 则用 Gateway。

## 2. Peer、Orderer、CA、invoice 链码分别是什么？

### Peer（对等节点）

Peer 是保存账本、执行链码的业务节点。课程网络中有 `peer0.org1.example.com` 和 `peer0.org2.example.com`。

后端提交“创建发票”时，Peer 会执行 `CreateInvoice`，检查发票 ID 是否重复、操作人是否符合规则等；交易成功后，Peer 保存区块账本和最新世界状态。可以把它理解为“共同记账并执行规则的机构服务器”。

### Orderer（排序节点）

Orderer 不判断发票业务是否正确，它负责把已经通过 Peer 背书的交易按统一顺序排列、打包成区块，再发给 Peer 提交。

它解决的问题是：多个节点不能各自决定“交易先后顺序”，否则账本可能不一致。可以理解为“交易队列管理员和区块打包员”。

### CA（Certificate Authority，证书机构）

CA 是网络的身份签发机构，类似“发身份证的机构”。它为组织、Peer 和用户生成证书与私钥。

当前项目后端使用的是 Org1 的测试用户：

```text
User1@org1.example.com
```

Peer 可通过 CA 签发的证书验证交易签名是否可信。

### invoice 链码

链码就是部署在 Fabric 上的业务规则程序，代码在 `chaincode/invoice_contract.go`。它定义本项目的 `CreateInvoice`、`TransferInvoice`、`VerifyInvoice` 等函数。

Fabric 规定“怎样读写账本”；我们定义“什么是创建发票、什么是发票流转”。链码规则不能被用户仅靠修改网页绕过。

## 3. Docker 在这里具体做什么？

Fabric 网络由多个服务组成，并非一个单独程序。Docker 用容器启动这些服务：

```text
peer0.org1.example.com  → Org1 的账本和链码执行节点
peer0.org2.example.com  → Org2 的账本和链码执行节点
orderer.example.com     → 排序和出块节点
ca_org1 / ca_org2       → 测试证书机构
```

`./scripts/start-network.sh` 会调用课程资源中的 `test-network/network.sh`，由它使用 Docker 创建网络、生成测试证书、创建通道。

网页与 Go 后端本身不在 Docker 中运行，但 Go 后端要连接 Docker 容器里的 Peer，所以先启动 Docker/Fabric 是必要的。可用 `docker ps` 查看容器是否存在。

## 4. `mychannel` 是什么？

`mychannel` 是 Fabric 的“通道（Channel）”名称。通道可理解为：**指定成员组织共同维护的一本独立账本**。

```text
Org1 的 Peer ─┐
              ├── mychannel：本项目的发票账本
Org2 的 Peer ─┘
```

同一个 Fabric 网络可拥有多个通道，例如发票、物流和结算可以分别拥有账本。课程 Test Network 默认使用 `mychannel`，所以本项目沿用它。

后端通过以下代码定位链码：

```go
gateway.GetNetwork("mychannel").GetContract("invoice")
```

意思是：获取 `mychannel` 通道，再获取其中名为 `invoice` 的链码。

## 5. 几个脚本运行的顺序是什么？

从完全关闭状态启动，顺序必须是：

```bash
cd /home/wenqin/bc/blockchain/project/invoice-ledger

# 1. 启动 Fabric 容器、创建 mychannel
./scripts/start-network.sh

# 2. 将发票链码部署到 mychannel
./scripts/deploy-chaincode.sh

# 3. 启动 Go 后端和网页
./scripts/start-app.sh
```

依赖关系是：

```text
Fabric 网络未启动 → 没有 Peer、证书和 mychannel
               → 不能部署链码
               → 后端也不能连接 invoice 链码
```

每一步都要确认没有报错再运行下一步。第三步会持续运行服务，显示 `http://localhost:8080` 后请不要关闭该终端。

## 6. Fabric 有什么固定接口吗？

有，分为两层。

### Fabric 固定提供的链码 API

链码可使用 Fabric 提供的接口读写账本：

```go
ctx.GetStub().GetState(key)           // 读取当前世界状态
ctx.GetStub().PutState(key, value)    // 写入当前世界状态
ctx.GetStub().GetStateByRange(...)    // 范围查询
ctx.GetStub().GetHistoryForKey(key)   // 查询一个 key 的历史
ctx.GetStub().GetTxID()               // 当前交易 ID
ctx.GetStub().GetTxTimestamp()        // 当前交易时间
```

### 项目自定义的业务接口

`CreateInvoice`、`TransferInvoice` 不是 Fabric 自带的，而是我们在链码中自己写的业务函数。网页则调用我们自己定义的 HTTP 接口：

```text
POST /api/invoices                 创建发票
GET  /api/invoices                 查询全部发票
POST /api/invoices/:id/transfers   流转发票
POST /api/invoices/:id/verify      核验发票
```

## 7. 发票流转是什么？

发票流转指发票在后续业务中由一个处理主体交给下一个处理主体的过程。例如：

```text
星河科技开具发票
    ↓
云帆贸易收到，成为当前持有人
    ↓
云帆贸易交给北辰供应链处理结算
    ↓
北辰供应链交给财务审核部
```

在本项目中，每次流转会：

1. 更新 `CurrentHolder`；
2. 更新状态为 `IN_CIRCULATION`；
3. 新增一条独立 `InvoiceFlow` 流转记录；
4. 作为新的 Fabric 交易写入区块链。

它是课程项目抽象出的“责任/处理权可追溯交接”模型，并不自动代表法律意义上的所有权转移、税务抵扣或正式红冲。

## 8. “用户”有哪些不同概念？

至少要分清四种：

| 概念         | 当前项目中是什么           | 例子                       |
| ------------ | -------------------------- | -------------------------- |
| 浏览器使用者 | 正在打开网页、填写表单的人 | 你                         |
| 业务操作人   | 表单里的`operator`       | “云帆贸易财务”           |
| 业务主体     | 销售方、购买方、持有人     | “星河科技有限公司”       |
| Fabric 身份  | 真正给交易签名的证书用户   | `User1@org1.example.com` |

当前的真实过程是：

```text
你在网页填写“操作人 = 云帆贸易”
          ↓
后端统一用 User1@org1 的测试私钥签名
          ↓
Fabric 技术上识别为 Org1 的 User1
```

所以课程演示版并没有“每个企业分别登录并使用自己证书”的完整认证系统。生产环境会让每个企业/员工拥有独立 CA 证书，并由链码读取证书中的组织与角色做权限判断。

## 9. 具体讲讲“后端用 Fabric 证书签名交易”

后端读取 Org1 测试用户的两类文件：

```text
signcerts/  → 用户证书，可用于证明身份
keystore/   → 用户私钥，必须保密，用于生成签名
```

它们位于：

```text
test-network/organizations/peerOrganizations/org1.example.com/
users/User1@org1.example.com/msp/
```

当后端要调用 `CreateInvoice("INV-001", ...)` 时，Gateway 会用私钥对交易请求做数字签名：

- Peer 用证书中的公钥验证签名；
- 通过说明请求来自该私钥持有者；
- 若有人篡改交易参数，签名验证会失败；
- 没有私钥就不能伪造该身份的交易。

完整过程：

```text
Go 后端构造交易
    ↓
Gateway 使用 User1@org1 私钥签名
    ↓
Peer 验证证书并执行 invoice 链码，返回背书结果
    ↓
后端提交交易给 Orderer
    ↓
Orderer 排序、生成区块
    ↓
Peer 提交区块，更新账本和世界状态
```

签名证明的是“技术身份 `User1@org1` 发起请求”，并不自动证明网页中填写的销售方在现实世界中的法律身份。

## 10. 世界状态和区块历史是什么？怎么看？

### 世界状态

世界状态是业务对象当前最新快照。例如一张发票经历三次流转后，世界状态只需记录：

```text
当前持有人 = 财务审核部
```

网页首页的“发票账本”列表读取的就是世界状态，因此能快速显示每张发票当前状态。

### 区块历史

区块历史保存每一次已提交的状态变化。它能回答：这张发票以前由谁持有、什么时候流转、对应哪笔交易。

网页中查看方法：在“发票账本”找到一张发票 → 点击“详情” → 看“发票流转”与“链上历史”。

```text
发票流转：我们额外保存的业务时间线，适合人阅读
链上历史：Fabric 的底层键级历史，包含 TxID 和当时的状态
```

## 11. 我在浏览器填写信息时，算什么角色？

你首先是**浏览器使用者/系统操作员**，没有直接登录到 Fabric。

例如你填写：

```text
销售方：星河科技
操作人：星河科技
```

链码会将“星河科技”作为业务上的创建者，并检查创建时操作人是否等于销售方；但从 Fabric 技术角度，真正签名和提交交易的是后端统一使用的 `User1@org1.example.com`。

```text
你                 → 网页操作员
表单中的操作人      → 业务规则角色
User1@org1          → Fabric 网络看到的证书身份
```

## 12. 什么叫“持有人”？

持有人是当前负责保管、处理或继续流转该发票的业务主体。

当前模型中：销售方创建发票后，购买方默认成为初始持有人；当前持有人可以将发票转给下一主体，接收方成为新的持有人。

```text
销售方：星河科技
购买方：云帆贸易

创建后：当前持有人 = 云帆贸易
云帆贸易转给北辰供应链后：当前持有人 = 北辰供应链
```

它表示“当前有权处理和继续交接发票的主体”，不必严格等于法律所有权、税务抵扣资格或纸质发票实际保管人。链码以它作为流转规则：**只有当前持有人才能发起下一次流转。**

---

## 一句话总复习

```text
网页负责填写和查看；
Go 后端负责计算哈希、保管测试证书、调用 Gateway；
Gateway 带着证书连接 Fabric；
Peer 执行链码并保存账本；
Orderer 负责统一交易顺序和出块；
链码负责发票存证、流转和核验规则；
世界状态展示当前结果，区块历史保留完整变化过程。
```

---

## 13. 什么是 `gRPC 连接`？

gRPC 是 Google 开源的一种“远程过程调用（Remote Procedure Call）”通信方式。它的目的与 HTTP API 类似：让一个程序调用另一台机器上的程序功能；但它更适合后端服务之间高效、结构化地通信。

在本项目中有两种不同通信：

```text
浏览器  ←→  Go 后端       使用 HTTP / JSON
Go 后端 ←→ Fabric Peer    使用 gRPC / Protocol Buffers / TLS
```

例如你点击“创建发票”后：

1. 浏览器用 HTTP 向 `http://localhost:8080/api/invoices` 发送 JSON；
2. Go 后端解析 JSON、计算哈希；
3. Go 后端通过 gRPC 连接到 Fabric 的 `peer0.org1.example.com:7051`；
4. 后端通过该连接提交链码调用请求。

为什么不直接全部用 HTTP？因为 Fabric 的节点之间和 SDK 通信需要高效、固定的数据结构和双向能力，gRPC 通常使用 Protocol Buffers 二进制数据格式，性能和类型约束都比普通 JSON HTTP 更适合这类节点通信。

代码中的这句就是建立 gRPC 连接：

```go
grpc.NewClient(peerEndpoint, grpc.WithTransportCredentials(...))
```

其中 `peerEndpoint` 默认是：

```text
dns:///localhost:7051
```

`localhost:7051` 表示你的 Go 后端通过本机映射端口，连接到 Docker 容器中的 Org1 Peer；`TLS` 则保证连接过程被加密，并验证你连接的确实是可信 Peer。

## 14. `peer0.org1.example.com` 和 `peer0.org2.example.com` 有什么区别？

它们分别属于两个不同组织：

```text
peer0.org1.example.com  → Org1 的第 0 个 Peer
peer0.org2.example.com  → Org2 的第 0 个 Peer
```

名称可以拆开理解：

```text
peer0       → 这个组织里的第 0 个 Peer；以后可以扩展 peer1、peer2
org1/org2   → 所属组织不同
example.com → 课程网络使用的示例域名
```

课程 Test Network 把它们想象成两家不同机构的服务器即可。它们都加入了 `mychannel` 后，会共同保存该通道账本的副本；因此同一笔已提交发票交易最终会被两个 Peer 记录。

区别主要在于：

| 项目                  | Org1 Peer       |        Org2 Peer        |
| --------------------- | --------------- | :----------------------: |
| 所属机构              | Org1            |           Org2           |
| 使用的 CA / MSP       | Org1 的证书体系 |     Org2 的证书体系     |
| 保存的 mychannel 账本 | 一份副本        |         一份副本         |
| 当前项目后端连接      | 是，默认连接它  | 否，当前后端未直接连接它 |

当前 `backend/fabric/gateway.go` 使用的是 Org1 的 `User1@org1` 证书，所以默认连接 `peer0.org1`。Org2 Peer 仍然有意义：它代表另一家参与机构也保留账本并参与网络，而不是由 Org1 单独控制全部数据。

在更真实的项目中，可能会要求一笔重要交易同时得到 Org1 和 Org2 的 Peer 背书；是否需要由“背书策略”决定。

## 15. Orderer 只是排序吗？它会决定谁负责持有下一个区块吗？

你的理解基本正确：**Orderer 的业务职责是接收已经背书的交易、确定全网统一顺序、打包区块并分发。**它不执行发票链码，不决定谁是发票持有人，也不负责像比特币矿工那样竞争“谁挖到下一个区块”。

Fabric 的过程更准确地说是：

```text
Peer：执行链码，判断发票交易是否符合规则，产生背书
Orderer：将已背书交易排序，凑成一个区块
Peer：验证区块并把区块保存到自己的账本副本
```

因此，“下一个区块由谁持有”这种说法更接近某些公有链的直觉，但不完全适用于 Fabric：

- 区块由 Orderer 服务产生并广播；
- 加入该通道的每个 Peer 都会保存这个区块的副本；
- 没有一个 Peer 独占“下一个区块”；账本是多副本共享的。

Orderer 自身也可能由多个排序节点组成。多个 Orderer 之间会通过排序服务的共识机制协作，某个节点在某一时刻可能承担领导角色来提出顺序，但这是 Orderer 集群内部实现细节；它不等于比特币的挖矿竞争，也不改变“所有 Peer 都保存最终区块”的事实。

课程 Test Network 中通常只看到一个 `orderer.example.com`，所以你不会明显感受到多个 Orderer 的协调过程。

## 16. Peer 和用户之间是什么关系？

Peer 是运行在机构服务器上的 Fabric 节点；用户是持有证书和私钥、通过客户端发起请求的身份。两者不是同一个东西。

```text
用户（持有证书/私钥）
    ↓ 通过 Gateway 提交签名请求
Peer（验证身份、执行链码、保存账本）
```

以当前项目为例：

```text
User1@org1.example.com  → 一个 Org1 的测试用户身份
peer0.org1.example.com  → Org1 运行的 Peer 服务器
```

它们同属 Org1，但职责不同：

| 对比项           | 用户`User1@org1`    | Peer`peer0.org1`         |
| ---------------- | --------------------- | -------------------------- |
| 本质             | 一个数字身份          | 一个运行中的网络服务/节点  |
| 是否有证书       | 有                    | 有                         |
| 私钥作用         | 对交易请求签名        | 对节点通信和背书等操作签名 |
| 是否执行链码     | 否                    | 是                         |
| 是否保存账本     | 否                    | 是                         |
| 是否直接操作网页 | 后端代表它调用 Fabric | 不直接接触网页             |

完整关系是：浏览器用户操作页面 → 后端代表 `User1@org1` 用私钥签名 → Org1 Peer 验证该身份并执行链码。Peer 不是用户的“账号”，它更像用户提交业务请求所访问的可信账本服务器。

## 17. 现在这个系统中 Channel 只有一个吗？

是的，当前课程项目只有一个通道：

```text
mychannel
```

原因是启动脚本明确执行：

```bash
./network.sh up createChannel -c mychannel -ca
```

发票链码 `invoice` 也只部署在这个通道上。因此当前项目所有发票记录、流转记录和历史都属于：

```text
mychannel / invoice 链码
```

Fabric 技术上当然可以有多个通道。例如可将不同合作联盟拆开：

```text
invoice-channel     → 发票存证与核验
settlement-channel  → 企业间结算
audit-channel       → 审计业务
```

但每增加一个通道，都要额外创建通道、让指定 Peer 加入、部署对应链码，并处理不同通道的数据同步和权限设计。对当前课程作业而言，一个 `mychannel` 已经足够展示核心能力。

## 18. 待补充的问题

第 18 项目前没有具体内容。你写好问题后，我可以继续补充答案。
