# Windows 本地完整服务

在 `D:\XINIUNIAO` 双击 `启动WeKnora.cmd`。脚本检查 Docker Desktop，等待五个服务健康后打开 <http://127.0.0.1:5174>。首次构建需要数分钟；代码未变化时复用构建产物。启动入口使用命令行窗口。

| 入口 | 作用 |
| --- | --- |
| `启动WeKnora.cmd` | 启动完整服务并打开网页；可以重复执行 |
| `停止WeKnora.cmd` | 停止本地服务的五个容器，保留数据 |
| `WeKnora状态.cmd` | 查看容器运行状态和健康状态 |

服务属于 Docker Compose 项目 `weknora-personal`，包括网页服务、应用后端、PostgreSQL 数据库、Redis 缓存与任务队列、DocReader 文档解析服务。网页监听本机端口 5174，后端监听本机端口 18080，数据库和文档解析服务通过容器内部网络连接。容器配置了 `unless-stopped` 重启策略；Docker 引擎重新启动时会恢复此前运行的服务。

首次初始化从 `WeKnora-postgres-dev` 备份并复制 `WeKnora` 数据库，沿用 `WeKnora-topic3/.env` 中的配置和加密密钥，复制 `WeKnora-topic3/.local-data/files` 中的本地文件。此后网页使用独立副本，新增账号、模型配置、知识库和实验记录保存在本地服务中。模型供应商沿用复制时的配置。

数据库存储在 `weknora-personal_postgres` 卷中。项目目录下的 `.local-service` 保存私有环境配置、文件、数据库备份和构建产物，已加入 Git 忽略规则。初始化备份为 `.local-service/backups/source-development.dump`，代码构建前的备份为 `before-build-时间.dump`。数据库启动迁移使用当前代码的迁移校验流程；校验失败时启动报告错误。

初始化针对本机版本号为 103 的源库，按迁移文件的表达式重建四个检查约束，同时验证现有数据，再执行完整迁移校验。此步骤处理 PostgreSQL 备份恢复产生的数组类型转换表达式差异。当前服务的官方迁移版本为 91，项目迁移版本为 16。应用日志位于 `.local-service/logs/app.log`。

脚本以当前源码构建后端和网页。源文件内容哈希保存在 `.local-service/build-stamp.txt`；源代码变化触发重新构建。构建使用本机已有的 `weknora-nogit-go:1.26` 和 `node:22-alpine` 镜像，网页运行在本机已有的 WeKnora Nginx 镜像中。依赖安装可能需要联网。这套入口针对本机现有环境。

网页构建在 Linux 容器的临时目录中完成，成功后复制网页资源。后端构建写入提交身份和构建时间。构建失败时脚本尝试启动已有后端产物，并保留错误记录。项目根目录同时提供 `Start-WeKnora.cmd`、`Stop-WeKnora.cmd` 和 `WeKnora-Status.cmd`，便于从仓库目录启动、停止和查看状态。

命令行操作：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\personal-service.ps1 -NoBrowser
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\personal-service.ps1 -Action Status
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\personal-service.ps1 -Action Stop
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\personal-service.ps1 -Rebuild
```

在项目目录查看最近日志：

```powershell
docker compose --env-file .local-service/runtime.env -f docker-compose.personal.yml logs --tail 100 app docreader frontend
```

启动失败会保留窗口，简要错误写入 `.local-service/last-error.txt`。端口 5174 或 18080 被其他程序占用时，需要先处理端口冲突。初始化发现目标库已有应用表时会停止恢复，保留现场供检查。运行中的模型请求由现有模型配置决定；启动检查只访问服务健康接口和网页。
