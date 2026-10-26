# AP 历史统计接口兼容

接口以当前 AP `d0ce990caf24b821e4250909ada78016f95d31d0`、历史图片上传客户端 `cdbc5aef3dbdb113ebc285ea40e8e462cda2977b` 和原 `Alas_Cloud/go_backend` 为依据。科研出现记录和倾向识别参照 [AzurStats](https://github.com/LmeSzinc/azur-stats-server/blob/3a743f091b0a98d42a8dc04c3d16be9552a9c7d6/AzurStats/image/research_list.py)。

| 路径 | 请求与响应 |
|---|---|
| `POST /api/upload`、`/api/upload/` | A 接收 multipart `file`，可带 `device_id`、`genre`、`combat_count`；也支持 `User-Agent: Alas (<id>)` 的单文件客户端。成功 HTTP 200，包含 Lsky `status/data.md5` 和 Imgurl `code/imgid` 字段。 |
| `GET /api/data/opsi_items` | C 接收 `device_id`、`genre`、`limit`、`offset`，返回 `data` 数组，保留图片、海域、侵蚀等级、战斗数及物品标签。 |
| `POST /api/azurstat` | 原 JSON `device_id/task/body`；支持旧版大世界任务、物品与 `is_meow`。 |
| `GET /api/azurstat/{stats,items,history,filters}` | 原聚合结构及任务、海域、侵蚀等级、物品类别、日期筛选；`history` 支持 day/month。 |
| `POST /api/telemetry` | 原月度累计 CL1 字段，无需新增请求头。`net_stamina_gain` 表示原始明石行动力；派生值由服务端重算。 |
| `GET /api/telemetry/{stats,history,global_history}` | 原统计及月度历史结构；设备历史要求完整设备 ID。 |
| `GET /api/telemetry/stats/stream` | SSE，立即发送统计，变化后推送，30 秒心跳。 |
| `GET /api/get/announcement` | 根级 `announcementId/title/content/url`；无公告或同 ID 返回 HTTP 200 `{}`。 |
| `GET /api/force_update` | 根级 JSON 布尔值。 |
| `GET /api/maintenance` | `data.servers` 含各区服日期、时间及时区；保留 CN 顶层回退。 |

## 路由与部署

C 设置 `LEGACY_UPLOAD_PUBLIC_URL=http://160.202.238.171:30207/api/upload`，图片请求以 **307** 转交 A；A 设置 `STATISTICS_PUBLIC_URL=https://stats.azurpilot.com`，其他历史接口以 307 转交 C。客户端须跟随重定向并保留 POST 正文。浏览器 HTTPS 页面应使用正式 HTTPS 上传入口，不能依赖 HTTP IP 上传的混合内容请求。

A 的历史图片接口仍使用 64 KiB 流式落盘、哈希去重、暂存配额和两路云盘上传。旧客户端不提供文件大小，故每次仅预留单文件上限，失败释放空间。捕获时间优先使用显式 `collected_at`，其次采用 13 位毫秒时间戳文件名，缺失时使用接收时间。无 genre 截图尝试识别科研/大世界场景；不能可靠分类的进入待复核。

## 统计口径

A 按每小时 **:00/:30** 派发已上传完成的数据，不按 30 份触发。`BATCH_SIZE=30` 仅为消息容量；不足 30 份也发送。B 重跑模板不等待下一时段。科研出现率按每次刷新的项目展示次数计算，保存科研倾向；C 使用原始采集时间分桶，默认 `Asia/Shanghai`，项目筛选保持完整分母。`/api/v2/get/research/time-slots` 支持 `from/to/timezone/focus_series/project`。

CL1 每设备、实例、月份只保留一份累计快照，重传不累加；无采集时间的旧请求不能用更低累计值覆盖。旧仪表盘行动力成本按轮次 ×5，新 v2 保持自身口径。

无幂等标识的旧 JSON 掉落每次请求独立记账；不能按正文去重，否则两次相同真实掉落会被丢弃。可增加 `Idempotency-Key` 或 `request_id` 实现安全重传。同标识不同内容返回 409。JSON 自报与认证 OCR 保留独立来源，旧查询可读取两类，v2 不伪造截图来源；OCR 新版本替换当前统计贡献。

当前 AP 已关闭远端图片上传；服务端兼容不会自动启用客户端上传。旧域名切换、旧数据库迁移需另外执行，本文不表示已经迁移。现有 v2、管理接口及鉴权保持独立。
