# Gitea 极致轻量化部署方案(专注自动镜像同步、低资源)

> 以下所有配置项均已对照 Gitea 源码核实(modules/setting/mirror.go、queue.go、Dockerfile.rootless 等),
> 网上流传的部分配置(如 `[mirror] SKIP_TAGS`、`[repository] DISABLE_AUTO_GC`、`[queue] MIRROR_WORKERS`)
> 在 Gitea 中并不存在,写了也不生效,本方案已剔除。

## 一、方案核心优势

- **零依赖**:SQLite 单文件数据库,无需 MySQL/Postgres,一键启动
- **极致轻量**:官方 Rootless 镜像非 root 运行,空闲内存约 120–180MB,适配低配机、NAS、树莓派
- **核心能力保留**:完整的 Pull/Push 双向自动镜像同步(GitHub/GitLab/Gitee)
- **冗余功能关闭**:禁用 CI、包仓库等,进一步降内存、降 CPU
- **安全稳定**:非 root 运行、权限最小化、自动重启保可用

## 二、硬件最低配置要求

- 内存:最低 256MB(空闲约 120MB;仓库数超过 50 或仓库较大建议 512MB)
- CPU:单核 1GHz 以上即可
- 存储:1GB+ 空间(仅存 Git 仓库与单文件数据库)
- 系统:支持 Docker 的 Linux / NAS / ARM 设备

## 三、一键部署(Docker Compose)

### 1. 创建部署目录

```bash
mkdir -p /data/gitea && cd /data/gitea
# rootless 镜像固定以 uid 1000 运行,数据目录须对其可写
mkdir -p data config && chown -R 1000:1000 data config
```

### 2. 编写 docker-compose.yml(轻量化专用配置)

```yaml
services:
  gitea:
    image: docker.gitea.com/gitea:latest-rootless
    container_name: gitea-light
    restart: always
    environment:
      - TZ=Asia/Shanghai
      # ===== 镜像同步核心配置(GITEA__区段__键,每次启动都会覆盖 app.ini 对应项)=====
      - GITEA__MIRROR__DEFAULT_INTERVAL=10m      # 新镜像的默认同步周期(默认 8h)
      - GITEA__MIRROR__MIN_INTERVAL=10m          # 允许设置的最小周期
      - GITEA__QUEUE__MAX_WORKERS=2              # 限制所有后台队列并发,防抢占资源
      - GITEA__REPOSITORY__ENABLE_PUSH_CREATE_USER=true   # 允许 push 自动建仓(SSH 中转方案依赖)
      # ===== 关闭冗余功能 =====
      - GITEA__ACTIONS__ENABLED=false
      - GITEA__PACKAGES__ENABLED=false
      # ===== 仅首次初始化生效(写入初始 app.ini,之后改动请编辑 config/app.ini)=====
      - DISABLE_REGISTRATION=true                # 关闭公开注册
      - ROOT_URL=http://192.168.1.10:3000/       # 改成你的访问地址,影响克隆 URL 显示
      - SSH_DOMAIN=192.168.1.10
    ports:
      - "3000:3000"
      - "2222:2222"
    volumes:
      - ./data:/var/lib/gitea                    # 数据与仓库
      - ./config:/etc/gitea                      # app.ini 所在目录
    logging:                                     # 限制日志占用
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
    deploy:
      resources:
        limits:
          cpus: "0.5"
          memory: 384M
```

注意:

- 环境变量格式必须是 `GITEA__区段__键`(如 `GITEA__SERVER__DISABLE_SSH`),
  `GITEA__DISABLE_SSH` 这种不带区段的写法会被静默忽略。
- Rootless 镜像**只能使用内置 SSH 服务器**(镜像模板已固定 `START_SSH_SERVER = true`),
  监听端口默认 2222,与端口映射一致,无需额外配置。

### 3. 启动服务

```bash
docker compose up -d        # 后台启动
docker compose ps           # 查看运行状态
docker compose logs -f gitea
```

## 四、首次初始化配置

浏览器访问 `http://你的IP:3000`:

- **数据库**:SQLite3(镜像内默认,无需改动)
- **服务器域名/基础 URL**:与 `ROOT_URL` 保持一致
- **管理员账号**:设置自定义账号密码
- **SSH 服务器**:使用内置 SSH,端口 2222
- 勾选 **禁止用户自行注册**
- 点击安装即可完成初始化

## 五、核心功能:自动镜像同步(HTTPS,原生支持)

### 1. 拉取镜像(GitHub/GitLab/Gitee → 自建 Gitea,自动备份)

1. 新建仓库 → **迁移外部仓库**
2. 地址填 HTTPS 格式,如 `https://github.com/luoweb/gitea.git`;
   官方原版镜像填 `git@github.com:...` 会报"不是一个有效的 URL",打上第六节补丁后可直接填 SSH 地址
3. 私有仓库需在授权信息中填平台 Token:
   - GitHub:Fine-grained PAT,`Contents` 只读即可
   - GitLab:勾选 `read_repository` 的 PAT
   - Gitee:私人令牌
4. **必须勾选:此仓库为镜像仓库**
5. 创建完成后按周期自动同步(默认由 `DEFAULT_INTERVAL` 决定,本方案为 10 分钟),
   也可在仓库页面点"立即同步"手动触发

优势:全程自动,代码、分支、Tag 全自动同步备份。

### 2. 推送镜像(自建 Gitea → 第三方平台)

1. 进入仓库 → 设置 → 推送镜像
2. 添加 **HTTPS 格式**远端地址 + Token(需写权限,如 GitHub PAT `Contents` 读写)
3. 可勾选 **提交时同步**(SyncOnCommit,本地 push 后立即推送)
4. 本地提交后自动同步到 GitHub/Gitee/GitLab

## 六、SSH 同步方案(源码补丁,已在当前分支实现)

适用动机:HTTPS 访问 GitHub 不稳定(被墙)的环境下,SSH(22 端口)拉取更可靠。

官方原版 Gitea 的拉取/推送镜像只接受 http/https/git 地址:`IsMigrateURLAllowed` 白名单校验,
`git@github.com:...` 在表单直接报"不是一个有效的 URL"。本分支已按"最小侵入、独立新增"原则打上补丁:

|改动|文件|说明|
|---|---|---|
|新增|`modules/git/remote_ssh.go`|scp 风格地址(`git@host:path`)归一化为 `ssh://user@host/path`,全部新逻辑集中于此|
|+1 行|`modules/git/remote.go`|`ParseRemoteAddr` 入口调用归一化(Web/API 迁移、推送镜像共用此入口)|
|+1 处|`services/migrations/migrate.go`|`IsMigrateURLAllowed` 放行 `ssh` 协议,主机黑白名单对 SSH 依然生效|
|+1 行|`Dockerfile.rootless`|补装 `openssh-client`(官方 rootless 镜像没有 `ssh` 二进制,git 的 SSH 传输依赖它)|
|文案|`options/locale/locale_en-US.json`|迁移表单提示加入 SSH|

克隆与周期同步本身就是原生 `git clone --mirror` / `git fetch`,无需任何改动;
推送镜像(出站)走同一校验,同样直接支持 SSH 地址。
日常开发的入站方向(开发者 → Gitea)走 rootless 内置 SSH 服务器(2222),与本补丁无关。

### 1. 自建镜像(必须,官方镜像缺 ssh 客户端)

```bash
cd /path/to/gitea-src
docker build -f Dockerfile.rootless -t gitea:light-rootless .
# docker-compose.yml 中 image: 改为 gitea:light-rootless
```

### 2. 初始化 SSH 密钥(每台机器一次)

容器内 ssh 客户端按 **passwd 家目录**找密钥:`/var/lib/gitea/git/.ssh/`
(不是 `HOME` 指向的 `/var/lib/gitea/home`),该目录镜像启动时已建好并 chmod 700:

```bash
# 生成实例密钥(必须无口令,容器内无 tty 输入口令)
docker exec gitea-light sh -c 'ssh-keygen -t ed25519 -N "" -f /var/lib/gitea/git/.ssh/id_ed25519 -C gitea-mirror'
# 固定上游主机公钥(必须,否则首次连接因无法交互确认而失败)
docker exec gitea-light sh -c 'ssh-keyscan -t ed25519 github.com >> /var/lib/gitea/git/.ssh/known_hosts'
# 查看公钥
docker exec gitea-light cat /var/lib/gitea/git/.ssh/id_ed25519.pub
```

### 3. 上游授权

- **拉取镜像**:公钥加为上游仓库 **Deploy Key(只读)**;GitHub 部署密钥保持不勾选写权限
- **推送镜像**:公钥加为上游 **Deploy Key(勾选 Write access)**

### 4. WebUI 使用

- 新建仓库 → 迁移外部仓库 → 服务类型选 **Git** → 地址直接填
  `git@github.com:owner/repo.git` 或 `ssh://git@host:port/owner/repo.git`
- `[migrations]` 的主机白名单/黑名单对 SSH 地址同样生效;周期同步与 HTTPS 镜像完全一致
- 带端口/IPv6 的地址请使用 `ssh://` 完整格式(scp 风格语法无法表达端口)

### 5. 不自建镜像时的变通:宿主机中转

继续使用官方镜像且上游仅 SSH 可达时,在宿主机(NAS 计划任务)用密钥拉取、HTTPS+Token 推入 Gitea,
Gitea 容器内不持有任何私钥:

```bash
# /data/mirror-ssh/run.sh  — chmod 600 保护此文件
set -e
export GIT_SSH_COMMAND="ssh -i /data/mirror-ssh/id_ed25519 -o StrictHostKeyChecking=accept-new"
MIRROR_ROOT=/data/mirror-ssh/repos
GITEA_URL=http://gitea-user:YOUR_TOKEN@127.0.0.1:3000

sync_repo() {   # $1=上游 SSH 地址  $2=目标仓库
  [ -d "$MIRROR_ROOT/$2.git" ] || git clone --mirror "$1" "$MIRROR_ROOT/$2.git"
  git -C "$MIRROR_ROOT/$2.git" remote update --prune
  git -C "$MIRROR_ROOT/$2.git" push --force "$GITEA_URL/$2.git" \
    "refs/heads/*:refs/heads/*" "refs/tags/*:refs/tags/*"
}

sync_repo git@gitlab.example.com:team/app.git app
```

```bash
*/30 * * * * /data/mirror-ssh/run.sh >> /data/mirror-ssh/run.log 2>&1
```

说明:首次 push 自动建仓(依赖第三节 `ENABLE_PUSH_CREATE_USER=true`);
push 地址用户名任意、密码位填 Gitea Access Token;该方向仍是单向备份,勿再对同一仓库开启推送镜像。

## 七、轻量化深度优化(低配机必做)

编辑 `config/app.ini`(与 Compose 环境变量互补,改后 `docker compose restart gitea`):

```ini
[log]
LEVEL = warn

; 拉取+推送镜像共用一个队列,降低并发防内存尖峰
[queue.mirror]
MAX_WORKERS = 2

; 大仓库同步超时放宽(默认 MIRROR=300s / MIGRATE=600s)
[git.timeout]
MIRROR = 600
MIGRATE = 600

; 每周日凌晨整理一次对象库;不要关闭,镜像仓库长期不 GC 会持续膨胀
[cron.git_gc_repos]
SCHEDULE = @every 168h
```

说明:

- 同步周期在 **仓库设置 → 镜像/推送镜像** 中可按仓库单独调整,最小值受 `MIN_INTERVAL` 限制
- 常见无效配置再次提醒:`SKIP_TAGS`、`DISABLE_AUTO_GC`、`MIRROR_WORKERS` 均不存在于 Gitea

## 八、日常维护命令

```bash
# 停止 / 重启
docker compose down
docker compose restart gitea

# 升级最新版
docker compose pull && docker compose up -d

# 备份(停机打包最稳妥:SQLite 直接热拷贝有损坏风险)
docker compose stop gitea
tar -czf "gitea-backup-$(date +%Y%m%d).tar.gz" -C /data gitea
docker compose start gitea

# 不停机备份的替代方案
docker exec gitea-light gitea dump --file /tmp/gitea-dump.zip
```

## 九、常见问题

|现象|原因与解决|
|---|---|
|填 `git@github.com:user/repo.git` 报"不是一个有效的 URL"|官方原版仅支持 http(s)/git 地址;打上第六节补丁后可直接填 SSH 地址,未打补丁时改用 https+Token 或宿主机中转|
|镜像没有按 10 分钟同步|新镜像才继承 `DEFAULT_INTERVAL`;老仓库在仓库设置中单独修改,且不能低于 `MIN_INTERVAL`|
|私有仓库拉取报鉴权失败|检查 Token 权限(GitHub: Contents 只读;GitLab: read_repository)|
|推送镜像失败|目标平台 Token 需写权限(GitHub: Contents 读写)|
|内存吃紧|调小 `[queue.mirror] MAX_WORKERS`、增大 `DEFAULT_INTERVAL`,或将 `memory` 限制提高到 512M|

## 十、方案对比

|部署方案|空闲内存|是否需数据库|镜像同步能力|适用场景|
|---|---|---|---|---|
|本轻量化 Gitea|120MB+|无需(SQLite)|完整双向自动同步|备份 GitHub/GitLab/Gitee、低配置设备|
|完整版 Gitea(带 CI)|400MB+|可选|完整|需要 CI/制品库的团队|
|Gogs|80MB+|无需|无自动镜像同步|无法满足同步需求|
|GitLab CE|4GB+|必须|拉取镜像可用,推送镜像需付费版|重度 DevOps 团队,不适合轻量化|

## 十一、关键注意事项

- **单向镜像原则**:同一仓库只做「第三方 → Gitea」或「Gitea → 第三方」一个方向,
  禁止双向自动同步,避免 Git 冲突
- SQLite 完全满足个人/小批量仓库同步,100 个仓库以内无性能压力
- 资源限制已预设,杜绝内存溢出;Token 权限按最小化原则只读/读写分离
- 所有镜像同步为 Gitea 原生能力,稳定、无插件、无第三方脚本依赖(方案 B 中转脚本除外)
