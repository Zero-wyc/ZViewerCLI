import { Socket } from 'socket.io-client';
export interface RoomSocketOptions {
    serverUrl: string;
    roomId: string;
    proxyUrl: string;
}
export declare function connectRoomSocket(options: RoomSocketOptions): Socket;
//# sourceMappingURL=socket.d.ts.map