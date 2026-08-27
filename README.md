# 压力测孔资格工作台

本项目面向风洞模型装调与测量团队，管理压力测孔从草拟基线修订、校准测量、缺陷返修、关联复测到独立放行的完整验收流程。系统以本地原子快照、只追加摘要链和不可覆盖资格证书保存证据，并提供原生单页浏览器工作台，无需 Node 构建链。

## 构建

```bash
go build ./...
```

## 运行

```bash
go run ./cmd/server -addr=127.0.0.1:19081
```

浏览器访问 `http://127.0.0.1:19081/`。数据默认保存在 `./data`，可通过 `-data` 指定其他目录。未传入 `-addr` 时，若 `PORT` 是有效端口号，服务绑定 `127.0.0.1:<PORT>`；否则使用 `127.0.0.1:19081`。服务只接受明确的回环监听地址。

## 测试与自检

```bash
go test ./...
go run ./cmd/server -self-check -addr=127.0.0.1:19081
```

自检使用临时数据目录和真实回环 HTTP 请求，执行一次泄漏判定、返修、两次连续合格复测、独立批准及证书摘要校验，完成后主动关闭服务。

## 业务规则

- 草拟状态可修订模型编号、试验目标、测孔清单和阈值方案；批次信息逐版保存字段级前后值、原因、操作人、时间和摘要，冻结同时锁定最新批次信息、清单与阈值摘要。
- 冻结前按草拟修订执行邻接拓扑预检：单向、未知、自引用和重复引用阻止冻结，孤立与跨区域关系必须逐项确认；冻结基线保存规范排序的邻接关系及确认事实。
- 校准履历不可覆盖，可追加带 SHA-256 证据的失准记录；引用失准校准的初始测量和复测会保留原值但不再支撑资格，并生成具体补测阻塞项。
- 单孔、批量测量和关联复测均按冻结拓扑保存逐相邻测孔响应；系统规范排序原值，以最不利通道计算串扰并在缺陷中保留触发测孔、比例和拓扑摘要。
- 轮次对比按测孔重排所有未作废且校准有效的初测与复测，展示压力比、衰减和最大相邻响应比的相邻变化，并区分正常、趋近阈值和恶化提示；提示不改变资格或送审规则。
- 批准前可用追加更正作废错录轮次；系统先检查处置和复测依赖，再回算测孔覆盖率、资格及连续合格进度，原始轮次始终保留。
- 缺陷处置以规范 SHA-256 证据摘要形成追加版本；返修任务可逐版分派责任技师、期限和优先级，逾期只作提示，非当前责任技师处置必须保存接手说明。
- 区域风险、覆盖率和缺陷分布从当前事实实时计算，可按区域、资格状态、缺陷类型和送审阻塞组合钻取。
- 测量质量统计按区域、轮次类型和测孔汇总有效轮次、覆盖率、合格率、缺陷计数及压力比、衰减和最不利相邻响应的最小值、最大值、平均值；作废、校准失准及冻结拓扑不匹配轮次不参与统计，结果保留批次修订、阈值版本、轮次集合和稳定摘要。
- 送审前按覆盖率、校准、有效轮次、测孔资格、开放缺陷和连续合格次数一次返回全部阻塞及定位；确认提交必须携带事务内复算一致的预检摘要。
- 独立复核退回会按清单项和测孔建立可追踪要求；处置证据及关联复测可显式映射要求，只有当前处置版本完成规定的连续合格复测后才自动完成，未完成要求及其稳定摘要纳入送审预检。
- 送审快照固定批次信息、阈值版本、有效轮次、校准引用和最早有效期；再次送审生成带前后快照摘要的规范差异包，批准必须确认当前差异摘要。
- 批次信息、阈值和草拟测孔清单均可只读查询连续修订历史，并在任意两个同类版本间生成规范字段差异；冻结摘要在查询时复算，查询不会产生审计事件或改变批次修订。
- 待复核页会按候选复核员列出建档、测量和返修参与依据，批准和退回事务再次执行同一独立性规则。
- 批准后可下载稳定的规范 JSON 证书；证书台账支持筛选和稳定分页，批量核验逐项报告内容、快照、审计链和输入摘要且全程只读。

## 主要扩展接口

- `POST /api/batches/{batchID}/baseline/revisions`：提交整份草拟测孔清单及变更原因。
- `POST /api/batches/{batchID}/info/preflight` 与 `/info/revisions`：预检并追加模型编号、试验目标修订。
- `GET /api/batches/{batchID}/baseline/topology/preflight`：按当前修订只读预检邻接拓扑。
- `POST /api/batches/{batchID}/thresholds/preflight` 与 `/thresholds/revisions`：预检并确认追加阈值方案版本。
- `POST /api/batches/{batchID}/measurements/batch/preflight`：批量测量只读预检。
- `POST /api/batches/{batchID}/measurements/batch`：确认后原子提交批量测量。
- `GET /api/batches/{batchID}/drift`：按测孔、区域、轮次类型和提示等级查询轮次对比。
- `GET /api/batches/{batchID}/statistics`：按 `tap_id`、`surface_zone`、`round_kind` 和 `level` 组合筛选测量质量统计快照。
- `GET /api/batches/{batchID}/history`：按 `history_type` 查询修订历史，可用 `from_version`、`to_version` 生成同类版本差异。
- `POST /api/batches/{batchID}/calibrations/{calibrationRef}/invalidate`：登记校准失准并隔离受影响轮次。
- `GET|POST /api/batches/{batchID}/measurements/{roundID}/void[/preflight]`：预检依赖并作废错录轮次。
- `POST /api/batches/{batchID}/defects/batch-treatment[/preflight]`：预检并原子确认缺陷批处置。
- `POST /api/batches/{batchID}/defects/{defectID}/assignment` 与 `GET /defect-tasks`：追加责任分派并查询任务状态。
- `POST /api/batches/{batchID}/retests/batch[/preflight]`：预检并原子录入关联复测。
- `GET /api/batches/{batchID}/calibration/impacts`：查询校准履历及旧引用影响。
- `GET /api/batches/{batchID}?surface_zone=...&qualification_status=...&defect_type=...&blocking=true`：组合筛选测孔矩阵。
- `GET /api/batches/{batchID}/certificate`：下载批准批次的规范 JSON 证书。
- `GET /api/batches/{batchID}/submit/preflight` 与 `/reviewer/preflight`：执行只读送审和复核员独立性预检。
- `GET /api/batches/{batchID}/audit` 与 `/audit/download`：组合检索、分页、校验并下载规范审计链段。
- `GET /api/certificates` 与 `POST /api/certificates/verify/batch`：查询证书台账及执行只读批量核验。
