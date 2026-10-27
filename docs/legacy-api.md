# AP 客户端接口兼容

统一公开入口为 `https://api.azurpilot.com`；`https://stats.azurpilot.com` 保留兼容及管理后台入口。AP 的核心 URL 集中在客户端 `deploy/api_endpoints.py`。

仅以 AP 客户端实际调用和碧蓝统计局源码为依据：当前 AP `3148d0878ec9028be85d12d7b158c49853f0b173`、历史图片上传客户端 `cdbc5aef3dbdb113ebc285ea40e8e462cda2977b`，以及 [AzurStats ResearchList](https://github.com/LmeSzinc/azur-stats-server/blob/3a743f091b0a98d42a8dc04c3d16be9552a9c7d6/AzurStats/image/research_list.py)。

| 路径 | 来源及契约 |
|---|---|
| `GET /api/v1/servers/{region}_{server_id}` | AP `module/server_checker.py`：按最后一个下划线拆分复合 ID，支持 `cn_ios`、`cn_channel`；复用 v2 网关缓存与 `server.id/name/status`，兼容客户端无需新增请求头。 |
| `POST /api/upload`、`/api/upload/` | AP 历史 `module/statistics/azurstats.py`：multipart `file/device_id/genre/combat_count`；HTTP 200。也支持上游 Alas `User-Agent: Alas (<id>)` 单文件上传，兼容 Lsky `status/data.md5` 和 Imgurl `code/imgid` 回执。 |
| `GET /api/data/opsi_items` | AP 历史 `module/statistics/azurstats.py`：`device_id/genre/limit/offset`，返回 `data` 数组。保留 imgid、海域、侵蚀等级、战斗数及物品标签，仅查询认证 OCR 当前版本。 |
| `POST /api/telemetry` | 当前 `module/statistics/cl1_data_submitter.py`：原月度累计字段，无需新增请求头。`net_stamina_gain` 表示原始明石行动力，派生值由服务端重算。 |
| `GET /api/get/announcement` | 当前 `module/base/api_client.py`：根级 `announcementId/title/content/url`；无公告或同 ID 返回 HTTP 200 `{}`。 |
| `GET /api/force_update` | 当前 `deploy/git.py`：根级 JSON 布尔值。 |
| `GET /api/maintenance` | 当前 `module/handover/handover.py`：`data.servers` 含各区服日期、时间及时区；保留 CN 顶层回退。 |

## 部署和上传

C 设置 `LEGACY_UPLOAD_PUBLIC_URL=http://160.202.238.171:30207/api/upload`，图片请求以 **307** 转交 A；A 设置 `STATISTICS_PUBLIC_URL=https://api.azurpilot.com`，其余兼容接口以 307 转交 C。客户端须跟随重定向并保留 POST 正文。浏览器 HTTPS 页面应使用正式 HTTPS 上传入口。

公告和更新策略 Worker 同时绑定新旧主机的 `/api/v2/get/notice/*`、`/api/v2/get/update/*`，C 发布回读使用 `api.azurpilot.com`。更新文档保留 `force_update` 字段；不恢复已取消的 `/api/updata` 总开关。

历史图片接口仍受 A 的流式落盘、64 KiB 缓冲、暂存配额、哈希去重及两路云盘上传限制。旧客户端不提供文件大小，每次仅预留单文件上限，失败释放空间；123 盘确认上传后删除本地原图。

采集时间优先使用显式 `collected_at`，其次采用 PNG 文件名的 13 位毫秒时间戳，缺失时使用接收时间。B 保留原始时间、海域和 meow/log/scan 标签；不同奖励帧的相同大世界物品累加。无 genre 截图尝试识别科研/大世界场景，无法可靠分类时进入待复核。

## 半小时时段与概率

A 在每小时 **:00/:30** 派发已上传完成的数据，不足 30 份也发送。`BATCH_SIZE=30` 仅限制单条消息容量。B 模板重识别无需等下一批，C 新版本替换旧贡献，重传不重复统计。

科研刷新保存五个项目及科研倾向，倾向识别参照 AzurStats 的图像检查与多数期数规则。C 按**实际采集时间**分桶，默认 `Asia/Shanghai`；上传、派发和识别延迟不会改变归属时段。项目筛选保持完整分母。

示例：`GET /api/v2/get/research/time-slots?focus_series=9&timezone=Asia%2FShanghai`；可增加带时区的 `from/to`、`project`、`series`、`scene`。返回 30 分钟时段内的展示次数及出现比例。

CL1 每设备、实例、月份只保留一份累计快照；无采集时间的旧请求不能用更低累计值覆盖。现有 v2 接口保持独立。

当前 AP 已关闭远端图片上传；服务端兼容不会自动启用客户端上传。旧域名切换和旧数据库迁移未包含在本次部署中。
