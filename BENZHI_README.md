# BENZHI_README

## 项目说明
- 项目：benzhi-project-92b341c3-4762-4126-adb6-2de78e30b98a
- 项目用途：已完整实现面向风洞试验团队的压力测孔连通性验收工作台，覆盖冻结基线、校准与原始测量、确定性缺陷判定、返修和连续复测、独立复核、不可变资格证书、审计摘要链及证书校验。
- Go 工具链：`golang:1.22`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 项目描述
- 项目名称：pressure-tap-qualification
- 项目介绍：面向风洞试验团队的压力测孔连通性验收工作台，对模型测孔基线、校准测量、缺陷返修、复测结论和独立放行进行可追溯治理，最终形成可校验的不可变试验资格证书。
- 项目概述：面向风洞试验团队的压力测孔连通性验收工作台，对模型测孔基线、校准测量、缺陷返修、复测结论和独立放行进行可追溯治理，最终形成可校验的不可变试验资格证书。
- 核心工作流：验收负责人建立模型测孔批次并冻结测孔清单，测量工程师登记校准依据和逐孔连通性测量，系统按规则标记堵塞、泄漏、串扰或缺测并暂停放行；技师关联缺陷实施返修，测量工程师完成针对性复测，全部测孔满足覆盖率与连续合格要求后，由未参与测量和返修的复核员独立批准或退回，批准后封存资格证书并允许按摘要校验。
- 对外接口：Go 服务提供无需 Node 构建链的原生单页浏览器工作台及仅供该页面调用的同源 JSON 端点；页面以批次状态、测孔矩阵、缺陷队列、复测记录、复核区和证书校验区完成主流程。服务支持 -addr=127.0.0.1:<port>，也支持从 PORT 读取端口并绑定 127.0.0.1:<PORT>，命令行参数优先，默认监听 127.0.0.1:19081，绝不默认绑定 0.0.0.0 或常见低位端口。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/server -self-check -addr=127.0.0.1:19081

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-92b341c3-4762-4126-adb6-2de78e30b98a-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-92b341c3-4762-4126-adb6-2de78e30b98a-arm64 linux/arm64

docker run -it benzhi-project-92b341c3-4762-4126-adb6-2de78e30b98a-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -self-check -addr=127.0.0.1:19081`
