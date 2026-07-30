package online.hcraft.hscrens.video

import android.media.MediaCodec
import android.media.MediaFormat
import android.os.Build
import android.os.Handler
import android.os.HandlerThread
import android.util.Log
import android.view.Surface
import java.nio.ByteBuffer
import java.util.concurrent.ConcurrentLinkedQueue

class VideoDecoder(private val surface: Surface, private val width: Int, private val height: Int) {
    private var codec: MediaCodec? = null
    private val nalQueue = ConcurrentLinkedQueue<ByteArray>()
    private val availableInputBuffers = ConcurrentLinkedQueue<Int>()
    private var handlerThread: HandlerThread? = null
    private var handler: Handler? = null

    fun start(sps: ByteArray, pps: ByteArray) {
        val format = MediaFormat.createVideoFormat(MediaFormat.MIMETYPE_VIDEO_AVC, width, height)
        format.setByteBuffer("csd-0", ByteBuffer.wrap(sps))
        format.setByteBuffer("csd-1", ByteBuffer.wrap(pps))
        if (Build.VERSION.SDK_INT >= 30) {
            format.setInteger(MediaFormat.KEY_LOW_LATENCY, 1)
        }
        format.setInteger(MediaFormat.KEY_PRIORITY, 0)

        codec = MediaCodec.createDecoderByType(MediaFormat.MIMETYPE_VIDEO_AVC)

        handlerThread = HandlerThread("DecoderThread").apply { start() }
        handler = Handler(handlerThread!!.looper)

        codec?.setCallback(object : MediaCodec.Callback() {
            var outputFrameCount = 0

            override fun onInputBufferAvailable(codec: MediaCodec, index: Int) {
                availableInputBuffers.offer(index)
                drainQueue()
            }

            override fun onOutputBufferAvailable(codec: MediaCodec, index: Int, info: MediaCodec.BufferInfo) {
                outputFrameCount++
                if (outputFrameCount % 30 == 0 || outputFrameCount == 1) {
                    Log.i("VideoDecoder", "🟢 onOutputBufferAvailable chamado! Frame #${outputFrameCount}. Renderizando (release=true)")
                }
                try {
                    codec.releaseOutputBuffer(index, true)
                } catch (e: Exception) {
                    Log.e("VideoDecoder", "Error releasing output buffer: ${e.message}")
                }
            }

            override fun onError(codec: MediaCodec, e: MediaCodec.CodecException) {
                Log.e("VideoDecoder", "Codec Error (isRecoverable=${e.isRecoverable}, isTransient=${e.isTransient}): ${e.diagnosticInfo}", e)
            }

            override fun onOutputFormatChanged(codec: MediaCodec, format: MediaFormat) {
                Log.i("VideoDecoder", "Output format changed: $format")
            }
        }, handler)

        codec?.configure(format, surface, null, 0)
        codec?.start()
    }

    private var startTimeUs = 0L

    fun queueNal(nal: ByteArray) {
        if (nalQueue.size > 30) {
            nalQueue.poll()
        }
        nalQueue.offer(nal)
        handler?.post { drainQueue() }
    }

    private fun drainQueue() {
        val c = codec ?: return
        while (!nalQueue.isEmpty() && !availableInputBuffers.isEmpty()) {
            val index = availableInputBuffers.poll() ?: break
            val nal = nalQueue.poll() ?: break
            try {
                val buffer = c.getInputBuffer(index) ?: continue
                buffer.clear()
                buffer.put(nal)
                
                val nowUs = System.nanoTime() / 1000
                if (startTimeUs == 0L) startTimeUs = nowUs
                val pts = nowUs - startTimeUs

                c.queueInputBuffer(index, 0, nal.size, pts, 0)
            } catch (e: Exception) {
                Log.e("VideoDecoder", "Error queuing input buffer: ${e.message}")
            }
        }
    }

    fun stop() {
        try {
            codec?.stop()
            codec?.release()
        } catch (e: Exception) {}
        codec = null
        handlerThread?.quitSafely()
        availableInputBuffers.clear()
        nalQueue.clear()
        startTimeUs = 0L
    }
}
