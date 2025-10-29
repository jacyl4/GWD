## system 包：`system.go` 行为解读

**核心职责**：收集并校验系统环境信息（CPU 架构、虚拟化类型、目录路径、必要命令），并提供若干便捷方法用于路径和环境判定。

### 数据结构
- **`SystemConfig`**：
  - `Architecture`: 架构（仅支持 `amd64`/`arm64`）
  - `VirtType`: 虚拟化类型（`physical`/`container`/`vm`）
  - `Branch`: 默认 `main`
  - `WorkingDir`: 默认 `/opt/GWD`
  - `TmpDir`: 默认 `/tmp`

### 初始化流程
- **`LoadSystemConfig()`**：
  1. 使用默认值初始化 `Branch`、`WorkingDir`、`TmpDir`。
  2. 调用 `detectArchitecture()` 识别架构：
     - 优先通过命令 `dpkg --print-architecture`。
     - 失败时回退到 `runtime.GOARCH`；仅接受 `amd64`/`arm64`，否则报不支持。
  3. 调用 `detectVirtualization()` 识别虚拟化环境：
     - 通过 `systemd-detect-virt` 获取类型；
     - 若为以下任一值则归类为 `container`：`openvz`、`lxc`、`lxc-libvirt`、`systemd-nspawn`、`docker`、`podman`、`proot`、`pouch`；
     - 其他非空值归类为 `vm`；
     - 调用失败（无命令或报错）则视为 `physical`。

### 环境校验
- **`(c *SystemConfig) Validate()`**：
  - 创建工作目录 `WorkingDir` 和临时目录 `TmpDir`（`0755`）。
  - 校验必要系统命令存在：`apt`、`wget`、`curl`、`systemctl`，缺失即报错。

### 常用便捷方法
- `GetRepoDir()`：返回仓库存放目录路径 `WorkingDir/.repo`。
- `GetLogDir()`：返回日志目录路径 `WorkingDir/logs`。
- `IsSupportedArchitecture()`：是否为受支持架构（`amd64`/`arm64`）。
- `IsContainer()`：是否判定为容器环境。
- `GetTempFilePath(filename)`：拼接 `TmpDir/filename`。
- `GetTempDirPath(dirname)`：拼接 `TmpDir/dirname`。

### 典型使用顺序（建议）
1. `cfg, err := LoadSystemConfig()` 获取配置和环境信息。
2. `err = cfg.Validate()` 确保目录与依赖命令就绪。
3. 根据 `cfg.IsSupportedArchitecture()`、`cfg.IsContainer()` 做分支逻辑。
4. 使用 `GetRepoDir()`、`GetLogDir()`、`GetTempFilePath()` 等组织文件路径。

### 错误处理策略
- 关键外部命令失败时进行降级：
  - 架构：`dpkg` 失败回退到 `runtime.GOARCH`；
  - 虚拟化：`systemd-detect-virt` 失败视为 `physical`；
- 对不支持的架构与缺失必需命令，直接返回错误，避免继续执行。


