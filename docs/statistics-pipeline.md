# 三节点统计系统

## 仓库和职责

项目目录下三个独立 Git 仓库分别为：

| 节点 | 仓库 | 持久化内容 |
|---|---|---|
| A，上传入口 | `AzurPilotUploadServer` | SQLite 哈希/文件/通知记录，未上传完的暂存图 |
| B，本地识别 | `AzurPilotStatsWorker` | SQLite 任务、结构化结果、模板版本、诊断 |
| C，当前仓库 | `AzurPillotCloudServer` | PostgreSQL 结果历史、当前统计贡献、候选小图、模板和管理状态 |

123 盘保存长期原图。A 云盘上传完成即删除本地副本；B 在处理结束/失败后删除工作图片，重试重新下载；C 不保存完整截图。A 不满 30 份就等待，模板重跑和试识别不受此门槛限制。

## 配置与部署顺序

三个服务均提供 GHCR 镜像，不需要在服务器上编译；各仓库复制 `.env.example` 为 `.env` 并配置后，执行 `docker compose pull` 和 `docker compose up -d`：

| 节点 | 镜像 |
|---|---|
| A | `ghcr.io/wess09/azurpilotuploadserver:latest` |
| B | `ghcr.io/wess09/azurpilotstatsworker:latest` |
| C | `ghcr.io/wess09/azurpillotcloudserver:latest` |

`main` 跟踪主分支，`sha-<完整提交号>` 固定内容；正式版本可使用版本标签。更新时重新拉取镜像并重建容器，保留数据卷。B 已包含固定客户端源码、模板和 CPU OCR 模型；Docker 部署不需宿主机 Alas/Python。只有原生运行 B 时才需要下面的 `ALAS_ROOT / ALAS_PYTHON` 配置。

1. C 按现有 Docker Compose 启动 PostgreSQL 和服务，配置 `ADMIN_TOKEN` 与独立的 `STATISTICS_TOKEN`。新 SQL 迁移随服务启动执行；升级前备份数据库。
2. B 按其 README 配置 `STATISTICS_URL`、相同 `STATISTICS_TOKEN`、123 盘开放平台凭据和可用的 `ALAS_ROOT`/`ALAS_PYTHON`。启动单进程，再用已有 Cloudflare Tunnel 暴露 HTTP/WebSocket 8091 端口，应用层使用 `WORKER_TOKEN` 鉴权。
3. A 配置相同 `WORKER_TOKEN`、B 的 `wss://.../internal/v1/batches` 和 123 盘凭据，启动两路上传进程。经过 Cloudflare Tunnel 时，客户端使用 `POST /api/v1/uploads/init` 登记完整元数据，逐文件 `PUT /api/v1/uploads/{upload_id}/files/{index}`，最后 `POST /api/v1/uploads/{upload_id}/complete`。每个图片请求最多 10 MiB，整组仍可达 500 MiB；具体重试协议见 [A 的上传文档](https://github.com/wess09/AzurPilotUploadServer#上传协议)。兼容的整组 multipart 请求需要允许至少 502 MiB 的 HTTPS 入口，不能直接经过免费套餐的 100 MB 请求上限。
4. C 若要按需查看原始帧，配置 `RECOGNIZER_URL=https://B域名`、`WORKER_TOKEN`。若 B 还经过 Cloudflare Access，必须让该服务请求通过相应 Access 策略。
5. 打开 `/admin/` 的“识别概览”“待分类模板”“模板库”，确认 B 在线，先试识别再发布。

单文件和单组大小限制可通过 `MAX_FILE_BYTES / MAX_GROUP_BYTES` 调整；三个节点应使用相同设置，默认分别为 10 MiB 和 500 MiB。A 的接收并发、云盘上传并发和暂存配额独立配置。

真实 123 盘、域名、Tunnel 和密钥由部署环境提供。不要将它们写入仓库。123 盘开放平台下载额度必须覆盖 B 的识别、重试和预览流量。

## A → B 协议 v1

A 建立受认证 WebSocket，发送以下消息。默认每批严格 30 项，B 允许 1–30 项以支持小批测试。消息最大 16 MiB，不含原图。

```json
{
  "type": "batch_ready",
  "protocol_version": 1,
  "batch_id": "0123456789abcdef0123456789abcdef",
  "items": [{
    "sha256": "图片组的64位小写哈希",
    "metadata": {
      "upload_id": "303d1f40-3280-4053-84dd-574a83f59d20",
      "device_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      "instance_id": "bbbbbbbbbbbbbbbb",
      "project": "research",
      "scene": "research_complete",
      "server": "cn",
      "collected_at": "2026-10-26T12:00:00+08:00",
      "file_sizes": [12345]
    },
    "files": [{"index": 0, "size": 12345, "sha256": "文件的64位小写哈希", "file_id": 123}]
  }]
}
```

组哈希是各文件 SHA-256 **二进制 32 字节摘要按索引拼接**后的 SHA-256。重排文件会改变组哈希，元数据不参与哈希；全局相同组只保留首次提交归属。B 验证索引、大小、组摘要并事务持久化后回复：

```json
{"type":"batch_accepted","batch_id":"0123456789abcdef0123456789abcdef"}
```

ACK 丢失时 A 重连后补发原批次 ID；B 对同 ID 不同正文拒绝接收，对相同批次重复确认。云盘文件确认完成前，组不能成为可派发样本。WSS 每 25 秒保活，A 在断开后重连。

## B → C 结果与回执

内部 API 需要 `Authorization: Bearer <STATISTICS_TOKEN>`，JSON 请求上限 8 MiB。`POST /internal/v1/statistics/results` 每次接受 1–100 个结果：

```json
{
  "results": [{
    "sha256": "与source相同的组哈希",
    "revision": 1,
    "status": "complete",
    "parser_version": "azurpilot-adapter-v1",
    "template_release": 1,
    "source": "这里使用上一节items中的完整对象",
    "events": [{"event_type":"complete","series":9,"project":"Q-268-MI","focus_series":0,"duration_seconds":3600,"items":[{"item":"Coins","amount":100,"tag":""}]}],
    "diagnostics": [], "candidate_ids": [], "used_templates": []
  }]
}
```

上例哈希和 `source` 为说明占位，实际必须提供合法哈希及对象。科研事件为 `refresh`（无掉落）或 `complete`，通用掉落为 `drop`。刷新每个出现项目是一条事件。状态为 `complete / needs_review / invalid`；只有 `complete` 参与正式统计。

HTTP 200 返回逐项 `receipts`，状态 `accepted / duplicate / conflict / invalid / retry`。B 仅对前两者完成任务，`retry` 保留结果重传，冲突/无效进入失败待查。`GET /internal/v1/statistics/results/{sha256}` 查询当前 `revision` 和状态，404 表示尚无记录。

C 按组哈希事务串行：初版从 1 开始，更正必须是当前版本 +1；同版本同内容幂等，同版本不同内容冲突。新版本在同一事务中删除旧统计贡献、写入新贡献，同时保留每版原始结构化结果。更正变为待复核时也会撤下旧贡献。CL1 原有接口和存储独立。

## 统计查询

- `GET /api/v2/get/research`
- `GET /api/v2/get/drop`

可选参数 `from / to` 为带时区 RFC3339，区间 `[from,to)`，默认最近 30 天，最大 366 天；支持 `server / scene / series / project` 筛选。返回 `groups / items / daily`；每天按 UTC 分组。查询返回当前有效版本，月度查询使用自然月起止时间，不会把历月重复累加。

`groups.appearance_rate` 为科研出现次数占筛选后全部出现次数的比例。物品 `amount` 是总数量，`occurrences` 是出现该物品的完成事件数，`samples` 包含未掉该物品的完成事件，`drop_rate=occurrences/samples`，`average_per_sample=amount/samples`。`amount_per_hour` 仅计算有时长的事件；无可用时长返回 null。物品标识来自模板名称，显示名称、物品所属期数与科研项目期数分别存储。

## 模板闭环与管理 API

B 自动提取未知图标，小图内容 SHA-256 为候选 ID；相同来源/文件帧/槽位重传不增加次数。候选 PNG 为 8–256 像素、最多 128 KiB；目前 B 提取 49×49 内部图标。C 只保存小图，人工确认名称和类别，支持同物品多个外观。

| 管理操作 | API（均需 ADMIN_TOKEN） |
|---|---|
| 识别概览、最近结果 | `GET /api/v1/admin/statistics/overview`、`/statistics/results` |
| 候选、模板、版本、指令 | `GET /api/v1/admin/template-candidates`、`/templates`、`/template-releases`、`/worker-commands` |
| 分类或忽略 | `PATCH /api/v1/admin/template-candidates/{id}` |
| 发布、回滚 | `POST /api/v1/admin/templates/publish`、`/templates/rollback` |
| 草稿试识别、重跑 | `POST /api/v1/admin/templates/test`、`/statistics/reprocess`，正文 `{"ids":["样本或候选哈希"]}` |
| 原始帧流式预览 | `GET /api/v1/admin/statistics/preview/{sha256}/{index}?frame=0` |

分类正文为 `{"status":"assigned","template":{"item_id":"Coins","name":"物资","category":"consumable","rarity":0,"series":0,"scene":"all","enabled":true}}`；忽略用 `status=ignored`。类别为 `ship_blueprint / equipment_blueprint / consumable / other`；模板场景 `all` 可跨科研和通用掉落使用，也可限制为 `research / drop`。

发布冻结整个模板包和 SHA-256，切换活动版本并登记待复核及使用过在线模板的样本重跑。回滚复制旧内容生成新版本，保持版本号递增；草稿不随回滚改动。B 通过 `POST /internal/v1/worker/sync` 每 30 秒上报心跳/指令进度并获取活动版本和指令，通过 `GET /internal/v1/template-releases/{id}` 下载并验包。每个识别任务固定模板快照，试识别不写正式结果。

## 验证

C：`TEST_DATABASE_URL=<专用PG> go test -race -count=1 ./...`、`go vet ./...`、`npm --prefix web run build`。A/B 各自 README 提供单测和启动命令。可选 B→C 联调在 B 仓库运行 `tests/test_c_integration.py`，需要全新专用 C 数据库、`STATISTICS_TEST_URL / STATISTICS_TEST_TOKEN / STATISTICS_TEST_ADMIN` 及 Alas 环境；使用真实 OCR 与真实 C/PostgreSQL，仅替换云盘下载。

实际 123 盘与 Tunnel 需填写部署凭据后另行验收；本地测试不会自动调用真实云盘或公开服务端口。

A 仓库还提供可选 `TestRealWorkerWebSocketContract`，通过专用 B 实例验证 30 份上传后的真实 WebSocket 派发、鉴权和持久化确认；云盘上传仍使用磁盘校验替身。
