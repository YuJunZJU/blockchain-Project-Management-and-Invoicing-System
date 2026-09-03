# 阿里云 OCR 与 AI 纠偏接入说明

本项目已经实现以下流程：

```text
上传图片 / PDF / OFD
        ↓
阿里云 RecognizeAllText（统一识别，Type=Invoice）
        ↓
规则校验（日期、必填字段、金额 + 税额 = 价税合计）
        ↓
可选：AI 纠偏建议（不会自动采用）
        ↓
用户确认、修改并提交发票存证
        ↓
最终确认的数据计算 SHA-256 后写入 Fabric
```

## 已实现的安全边界

- 原始发票文件只在 OCR 请求期间读取，不写入 Fabric，也不会保存在本项目服务器。
- AccessKey 与 AI Key 均只存在本机 `.env` 文件，浏览器和 GitHub 看不到它们。
- AI 只收到 OCR 已提取出的结构化字段与规则提示，不直接读取原始文件。
- AI 只返回“建议字段 + 修改理由”；用户点击“应用建议”后仍可以编辑，只有点击“生成存证并上链”才真正提交。
- OCR 的购买方名称不会被直接当作链上接收人账号。系统只会在成员名录中找到唯一同名、已启用的流转员时，才自动填入其账号。

## 你需要配置的内容

1. 在阿里云开通“文字识别 / 票据凭证识别”的**增值税发票识别**能力。
2. 创建 RAM 用户或使用已有 RAM 用户，并授予最小必要权限 `ocr:RecognizeAllText`；课程演示也可暂时使用官方策略 `AliyunOCRFullAccess`。
3. 创建 AccessKey，保存 `AccessKey ID` 与 `AccessKey Secret`。Secret 只会在创建时完整展示一次。
4. 在项目根目录执行：

   ```bash
   cp .env.example .env
   ```

5. 用编辑器打开 `.env`，填写：

   ```bash
   ALIYUN_ACCESS_KEY_ID=你的AccessKeyID
   ALIYUN_ACCESS_KEY_SECRET=你的AccessKeySecret
   ALIYUN_OCR_REGION=cn-hangzhou
   ```

6. 重启后端：

   ```bash
   ./scripts/start-app.sh
   ```

之后打开“发票存证”，选择发票文件并点击“开始识别”。支持 PNG、JPG、JPEG、BMP、GIF、TIFF、WebP、PDF，单个文件最大 10MB；PDF 默认识别第一页。

## 可选：配置 AI 纠偏

后端使用 OpenAI 兼容的 Chat Completions 协议，因此可接入阿里云百炼兼容模式或其他兼容服务。以百炼兼容模式为例，在 `.env` 补充：

```bash
AI_CORRECTION_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1
AI_CORRECTION_API_KEY=你的百炼APIKey
AI_CORRECTION_MODEL=qwen-plus
```

不配置这些 `AI_CORRECTION_*` 变量也不影响 OCR：系统仍会使用阿里云 OCR 和本地规则校验，只是不显示 AI 纠偏建议。

## 常见报错

- **OCR 服务尚未配置**：检查 `.env` 是否位于 `invoice-ledger/.env`，并重启后端。
- **无权限 / noPermission**：检查 RAM 用户是否有 `ocr:RecognizeInvoice` 权限，且已开通对应 OCR 服务。
- **文件格式或大小不支持**：换成清晰的图片或 PDF 首页，文件不超过 10MB；建议图片长宽都超过 500px。
- **金额校验不一致**：OCR 对模糊图片可能读错数字，请以原始发票为准并人工修改。
