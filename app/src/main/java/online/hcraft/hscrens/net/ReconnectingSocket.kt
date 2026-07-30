package online.hcraft.hscrens.net

import kotlinx.coroutines.*
import java.io.IOException
import java.net.InetSocketAddress
import java.net.Socket

class ReconnectingSocket(
    private val host: String,
    private val port: Int,
    private val onConnected: (Socket) -> Unit,
    private val onDisconnected: () -> Unit
) {
    private var job: Job? = null
    @Volatile private var currentSocket: Socket? = null

    fun start(scope: CoroutineScope) {
        job = scope.launch(Dispatchers.IO) {
            var backoffMs = 1000L
            while (isActive) {
                var connected = false
                try {
                    val socket = Socket()
                    currentSocket = socket
                    socket.tcpNoDelay = true
                    socket.connect(InetSocketAddress(host, port), 2000)
                    connected = true
                    backoffMs = 1000L
                    
                    onConnected(socket)
                } catch (e: Exception) {
                    // Trata qualquer desconexão ou erro de rede
                } finally {
                    if (connected) {
                        onDisconnected()
                    }
                }
                // Garante que SEMPRE haverá um intervalo antes da próxima tentativa
                delay(backoffMs)
                backoffMs = (backoffMs * 2).coerceAtMost(5000L)
            }
        }
    }

    fun stop() {
        job?.cancel()
        try {
            currentSocket?.close()
        } catch (e: Exception) {}
    }
}
