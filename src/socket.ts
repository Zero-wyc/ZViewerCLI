import { io, Socket } from 'socket.io-client'

export interface RoomSocketOptions {
  serverUrl: string
  roomId: string
  proxyUrl: string
}

export function connectRoomSocket(options: RoomSocketOptions): Socket {
  const { serverUrl, roomId, proxyUrl } = options

  const socket = io(serverUrl, {
    transports: ['websocket'],
    reconnection: true,
    reconnectionDelay: 1000,
    reconnectionDelayMax: 10000,
    // 标识为 CLI 代理，后端据此跳过浏览器用户的 token 鉴权，
    // 仅需在 cli-register 中提供 roomId 即可加入房间并广播代理地址。
    auth: { agent: 'zcontrol-cli' },
  })

  socket.on('connect', () => {
    console.log('[ZViewer CLI] Socket 已连接:', socket.id)
    // 向房间注册 CLI 代理
    socket.emit('cli-register', {
      roomId,
      proxyUrl,
      agent: 'zcontrol-cli',
      version: '0.1.0',
    })
  })

  socket.on('disconnect', (reason) => {
    console.log('[ZViewer CLI] Socket 断开:', reason)
  })

  socket.on('connect_error', (err) => {
    console.error('[ZViewer CLI] Socket 连接失败:', err.message)
  })

  // 接收房主播放状态，可用于本地日志/调试；实际进度同步仍由浏览器完成
  socket.on('watch-together-state', (state) => {
    console.log('[ZViewer CLI] 收到房主状态:', {
      currentTime: state.currentTime,
      isPlaying: state.isPlaying,
      sourceUrl: state.sourceUrl?.slice(0, 60),
    })
  })

  return socket
}
