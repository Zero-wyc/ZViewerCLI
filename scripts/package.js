#!/usr/bin/env node
/**
 * ZViewer CLI 跨平台打包脚本
 *
 * 使用 Node.js Single Executable Application (SEA) 技术，将 CLI 打包成
 * Windows (.exe)、macOS (x64/arm64) 和 Linux (x64) 的单文件可执行程序。
 *
 * 依赖：
 * - 已安装项目 devDependencies（esbuild、postject）
 * - 当前目录为 cli/
 */

const fs = require('node:fs')
const path = require('node:path')
const https = require('node:https')
const { execSync } = require('node:child_process')

const ROOT = path.resolve(__dirname, '..')
const RELEASE_DIR = path.join(ROOT, 'release')
const NODE_VERSION = process.version

const TARGETS = [
  {
    id: 'win',
    platform: 'win32',
    distPlatform: 'win',
    arch: 'x64',
    output: 'zviewer-cli-win.exe',
    startScript: 'start-cli.bat',
  },
  {
    id: 'macos-x64',
    platform: 'darwin',
    distPlatform: 'darwin',
    arch: 'x64',
    output: 'zviewer-cli-macos-x64',
    startScript: 'start-cli.sh',
  },
  {
    id: 'macos-arm64',
    platform: 'darwin',
    distPlatform: 'darwin',
    arch: 'arm64',
    output: 'zviewer-cli-macos-arm64',
    startScript: 'start-cli.sh',
  },
  {
    id: 'linux',
    platform: 'linux',
    distPlatform: 'linux',
    arch: 'x64',
    output: 'zviewer-cli-linux-x64',
    startScript: 'start-cli.sh',
  },
]

function run(cmd, opts = {}) {
  console.log(`> ${cmd}`)
  execSync(cmd, { cwd: ROOT, stdio: 'inherit', ...opts })
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest)
    https
      .get(url, { headers: { 'User-Agent': 'zviewer-cli-packager' } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          file.close()
          fs.unlinkSync(dest)
          return download(res.headers.location, dest).then(resolve, reject)
        }
        if (res.statusCode !== 200) {
          file.close()
          fs.unlinkSync(dest)
          return reject(new Error(`下载失败 ${url}: ${res.statusCode}`))
        }
        res.pipe(file)
        file.on('finish', () => {
          file.close(resolve)
        })
      })
      .on('error', (err) => {
        try {
          fs.unlinkSync(dest)
        } catch {}
        reject(err)
      })
  })
}

function ensureDir(dir) {
  if (!fs.existsSync(dir)) {
    fs.mkdirSync(dir, { recursive: true })
  }
}

async function buildBundle() {
  console.log('\n[1/4] 编译 TypeScript 并打包依赖...')
  run('npm run build')
  run('npx esbuild dist/index.js --bundle --platform=node --format=cjs --outfile=dist/bundle.cjs')
  run('node --experimental-sea-config sea-config.json')
}

async function packageTarget(target) {
  console.log(`\n[打包] ${target.id} (${target.distPlatform}-${target.arch})`)
  const outputPath = path.join(RELEASE_DIR, target.output)

  if (target.platform === 'win32') {
    fs.copyFileSync(process.execPath, outputPath)
  } else {
    const tarballName = `node-${NODE_VERSION}-${target.distPlatform}-${target.arch}.tar.xz`
    const tarballPath = path.join(RELEASE_DIR, tarballName)
    const extractDir = path.join(RELEASE_DIR, `tmp-node-${target.id}`)
    const nodeBinary = path.join(extractDir, 'bin', 'node')
    const url = `https://nodejs.org/dist/${NODE_VERSION}/${tarballName}`

    ensureDir(extractDir)
    if (!fs.existsSync(tarballPath)) {
      console.log(`  下载 Node.js 运行时: ${url}`)
      await download(url, tarballPath)
    }
    console.log('  解压 Node.js 运行时...')
    run(`tar -xf "${tarballPath}" -C "${extractDir}" --strip-components=1`)

    fs.copyFileSync(nodeBinary, outputPath)

    console.log('  清理临时文件...')
    fs.rmSync(extractDir, { recursive: true, force: true })
    fs.unlinkSync(tarballPath)
  }

  console.log('  注入应用代码...')
  run(
    `npx postject "${outputPath}" NODE_SEA_BLOB sea-prep.blob ` +
      `--sentinel-fuse NODE_SEA_FUSE_fce680ab2cc467b6e072b8b5df1996b2 ` +
      `--macho-segment-name NODE_SEA`
  )
}

function copyStartScripts() {
  console.log('\n[3/4] 复制一键启动脚本...')
  fs.copyFileSync(path.join(ROOT, 'start-cli.bat'), path.join(RELEASE_DIR, 'start-cli.bat'))
  fs.copyFileSync(path.join(ROOT, 'start-cli.sh'), path.join(RELEASE_DIR, 'start-cli.sh'))
}

function cleanup() {
  console.log('\n[4/4] 清理临时产物...')
  try {
    fs.unlinkSync(path.join(ROOT, 'sea-prep.blob'))
  } catch {}
}

async function main() {
  const requested = process.argv[2]
  const targets = requested
    ? TARGETS.filter((t) => t.id === requested)
    : TARGETS

  if (targets.length === 0) {
    console.error(`未知目标: ${requested}`)
    console.error(`可用目标: ${TARGETS.map((t) => t.id).join(', ')}`)
    process.exit(1)
  }

  ensureDir(RELEASE_DIR)
  await buildBundle()

  for (const target of targets) {
    await packageTarget(target)
  }

  copyStartScripts()
  cleanup()

  console.log('\n✅ 打包完成，产物位于 release/')
  const files = fs.readdirSync(RELEASE_DIR)
  files.forEach((f) => console.log(`   - ${f}`))
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
