#!/usr/bin/env node
/**
 * ZViewer CLI
 *
 * 本地高画质代理客户端：
 * - 连接 ZViewer 后端房间
 * - 启动本地 HTTP 代理，向浏览器提供高画质视频流
 * - 支持命令行直接连接，或启动 setup 网页进行扫码登录与一键填地址
 */
import { Command } from 'commander'
import { exec } from 'node:child_process'
import { platform } from 'node:os'
import { startCliAgent } from './agent'

const program = new Command()

program
  .name('zcontrol-cli')
  .description('ZViewer 本地高画质代理客户端')
  .version('0.1.0')

program
  .command('connect')
  .description('使用命令行参数直接连接房间并启动本地代理')
  .requiredOption('-s, --server <url>', 'ZViewer 后端地址，例如 http://localhost:3333')
  .requiredOption('-r, --room <id>', '房间 ID')
  .requiredOption('-c, --cookie <cookie>', 'B站 Cookie 字符串（含 SESSDATA）')
  .option('-p, --port <port>', '本地代理端口', '9333')
  .action(async (options) => {
    const agent = await startCliAgent({
      serverUrl: options.server,
      roomId: options.room,
      cookie: options.cookie,
      port: parseInt(options.port, 10),
      connectImmediately: true,
    })

    process.on('SIGINT', async () => {
      await agent.close()
      process.exit(0)
    })
  })

program
  .command('setup')
  .description('启动本地配置网页，支持扫码登录 B站 与一键填写房间地址')
  .option('-s, --server <url>', '预填后端地址，例如 http://localhost:3333')
  .option('-r, --room <id>', '预填房间 ID')
  .option('-p, --port <port>', '本地代理端口', '9333')
  .option('--no-open', '不自动打开浏览器')
  .action(async (options) => {
    const agent = await startCliAgent({
      serverUrl: options.server,
      roomId: options.room,
      port: parseInt(options.port, 10),
      connectImmediately: false,
    })

    const setupUrl = `${agent.proxyUrl}?server=${encodeURIComponent(
      options.server || ''
    )}&room=${encodeURIComponent(options.room || '')}`

    if (options.open) {
      openBrowser(setupUrl)
      console.log(`[ZViewer CLI] 已尝试打开浏览器: ${setupUrl}`)
    } else {
      console.log(`[ZViewer CLI] 请手动打开: ${setupUrl}`)
    }

    process.on('SIGINT', async () => {
      await agent.close()
      process.exit(0)
    })
  })

function openBrowser(url: string) {
  const cmd =
    platform() === 'win32'
      ? `start "" "${url}"`
      : platform() === 'darwin'
      ? `open "${url}"`
      : `xdg-open "${url}"`

  exec(cmd, (err) => {
    if (err) {
      console.error('[ZViewer CLI] 打开浏览器失败:', err.message)
    }
  })
}

program.parse(process.argv)
