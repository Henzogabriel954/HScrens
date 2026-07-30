package online.hcraft.hscrens.input

import android.annotation.SuppressLint
import android.view.MotionEvent
import android.view.SurfaceView
import java.io.IOException
import java.io.OutputStream
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.util.concurrent.Executors

enum class TouchAction(val code: UByte) {
    DOWN(0u), MOVE(1u), UP(2u), CANCEL(3u), POINTER_DOWN(4u), POINTER_UP(5u)
}

class TouchCapture(private val surfaceView: SurfaceView) {
    private val executor = Executors.newSingleThreadExecutor()
    var outputStream: OutputStream? = null

    init {
        setupListener()
    }

    @SuppressLint("ClickableViewAccessibility")
    private fun setupListener() {
        surfaceView.setOnTouchListener { view, event ->
            val action = event.actionMasked
            val pointerIndex = event.actionIndex

            when (action) {
                MotionEvent.ACTION_DOWN, MotionEvent.ACTION_POINTER_DOWN -> {
                    sendTouchPacket(
                        TouchAction.DOWN, event.getPointerId(pointerIndex),
                        normX(event.getX(pointerIndex), view.width),
                        normY(event.getY(pointerIndex), view.height)
                    )
                }
                MotionEvent.ACTION_MOVE -> {
                    for (i in 0 until event.pointerCount) {
                        sendTouchPacket(
                            TouchAction.MOVE, event.getPointerId(i),
                            normX(event.getX(i), view.width),
                            normY(event.getY(i), view.height)
                        )
                    }
                }
                MotionEvent.ACTION_UP, MotionEvent.ACTION_POINTER_UP -> {
                    sendTouchPacket(
                        TouchAction.UP, event.getPointerId(pointerIndex),
                        normX(event.getX(pointerIndex), view.width),
                        normY(event.getY(pointerIndex), view.height)
                    )
                }
                MotionEvent.ACTION_CANCEL -> {
                    for (i in 0 until event.pointerCount) {
                        sendTouchPacket(TouchAction.CANCEL, event.getPointerId(i), 0f, 0f)
                    }
                }
            }
            true
        }
    }

    private fun normX(x: Float, width: Int) = (x / width).coerceIn(0f, 1f)
    private fun normY(y: Float, height: Int) = (y / height).coerceIn(0f, 1f)

    private fun sendTouchPacket(action: TouchAction, pointerId: Int, xNorm: Float, yNorm: Float) {
        val stream = outputStream ?: return
        executor.execute {
            val buf = ByteBuffer.allocate(10).order(ByteOrder.LITTLE_ENDIAN)
            buf.put(action.code.toByte())
            buf.put(pointerId.toByte())
            buf.putFloat(xNorm)
            buf.putFloat(yNorm)
            try {
                stream.write(buf.array())
                stream.flush()
            } catch (e: IOException) {
                outputStream = null
            }
        }
    }
}
