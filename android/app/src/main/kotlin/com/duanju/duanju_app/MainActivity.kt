package com.duanju.duanju_app

import io.flutter.embedding.android.FlutterActivity
import android.app.UiModeManager
import android.media.AudioManager
import android.os.Build
import android.provider.Settings
import android.view.WindowManager
import android.os.Bundle
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.content.res.Configuration
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.net.Uri
import androidx.core.content.FileProvider
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import java.io.File

class MainActivity : FlutterActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            window.isNavigationBarContrastEnforced = false
            window.isStatusBarContrastEnforced = false
        }
    }

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "duanju/device")
            .setMethodCallHandler { call, result ->
                if (call.method == "deviceInfo") {
                    val mode = getSystemService(Context.UI_MODE_SERVICE) as UiModeManager
                    val television = mode.currentModeType == Configuration.UI_MODE_TYPE_TELEVISION ||
                        packageManager.hasSystemFeature(PackageManager.FEATURE_LEANBACK)
                    val version = packageManager.getPackageInfo(packageName, 0).versionName
                    result.success(mapOf("television" to television, "version" to version))
                } else if (call.method == "systemProxy") {
                    val connection = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
                    val proxy = connection.defaultProxy
                    val host = proxy?.host.orEmpty()
                    val address = if (host.isNotEmpty() && (proxy?.port ?: 0) > 0) {
                        "http://${if (host.contains(':')) "[$host]" else host}:${proxy!!.port}"
                    } else ""
                    result.success(mapOf("http" to address, "https" to address,
                        "bypass" to (proxy?.exclusionList?.toList() ?: emptyList<String>()),
                        "pac" to (proxy != null && proxy.pacFileUrl != Uri.EMPTY)))
                } else if (call.method == "networkType") {
                    result.success(networkType())
                } else if (call.method == "systemVolume") {
                    result.success(systemVolume())
                } else if (call.method == "setSystemVolume") {
                    val index = call.argument<Int>("index") ?: -1
                    result.success(setSystemVolume(index))
                } else if (call.method == "screenBrightness") {
                    result.success(screenBrightness())
                } else if (call.method == "setScreenBrightness") {
                    val value = call.argument<Double>("value") ?: -1.0
                    result.success(setScreenBrightness(value))
                } else if (call.method == "resetScreenBrightness") {
                    result.success(resetScreenBrightness())
                } else {
                    result.notImplemented()
                }
            }
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "duanju/updater")
            .setMethodCallHandler { call, result ->
                if (call.method == "installApk") {
                    val path = call.argument<String>("path")
                    if (path.isNullOrEmpty()) {
                        result.success("missing")
                        return@setMethodCallHandler
                    }
                    result.success(installApk(path))
                } else if (call.method == "openInstallSettings") {
                    result.success(openInstallSettings())
                } else {
                    result.notImplemented()
                }
            }
    }

    /** 是否已被系统授予「安装未知应用」权限。 */
    private fun canInstallPackages(): Boolean =
        Build.VERSION.SDK_INT < Build.VERSION_CODES.O || packageManager.canRequestPackageInstalls()

    /**
     * 系统媒体音量（0.7.5）。
     *
     * 播放器手势要调的是**系统媒体音量**，不是应用内音量；
     * 返回 {current, max}，由 Dart 侧按比例换算百分比。
     */
    private fun systemVolume(): Map<String, Int> {
        return try {
            val audio = getSystemService(Context.AUDIO_SERVICE) as AudioManager
            val max = audio.getStreamMaxVolume(AudioManager.STREAM_MUSIC).coerceAtLeast(1)
            val current = audio.getStreamVolume(AudioManager.STREAM_MUSIC)
            mapOf("current" to current, "max" to max)
        } catch (e: Exception) {
            mapOf("current" to 0, "max" to 1)
        }
    }

    /** 设置系统媒体音量档位（0.7.5）。越界自动收敛，不弹系统音量条。 */
    private fun setSystemVolume(index: Int): Boolean {
        return try {
            val audio = getSystemService(Context.AUDIO_SERVICE) as AudioManager
            val max = audio.getStreamMaxVolume(AudioManager.STREAM_MUSIC)
            val target = index.coerceIn(0, max)
            audio.setStreamVolume(AudioManager.STREAM_MUSIC, target, 0)
            true
        } catch (e: Exception) {
            false
        }
    }

    /**
     * 读取系统当前亮度比例（0~1，0.7.8）。
     *
     * 只读系统设置，**不需要任何权限**；用作手势的起点。
     */
    private fun screenBrightness(): Double {
        return try {
            val value = Settings.System.getInt(
                contentResolver,
                Settings.System.SCREEN_BRIGHTNESS,
            )
            (value / 255.0).coerceIn(0.0, 1.0)
        } catch (e: Exception) {
            0.5
        }
    }

    /**
     * 设置**本 App 窗口**亮度（0~1，0.7.8）。
     *
     * 用窗口亮度覆盖系统亮度：系统调得再暗，App 内也能拉亮，
     * 且不需要「修改系统设置」特殊权限；退出时会恢复跟随系统。
     */
    private fun setScreenBrightness(ratio: Double): Boolean {
        return try {
            val value = ratio.coerceIn(0.0, 1.0).toFloat()
            runOnUiThread {
                val attrs = window.attributes
                attrs.screenBrightness = value
                window.attributes = attrs
            }
            true
        } catch (e: Exception) {
            false
        }
    }

    /** 恢复窗口亮度为"跟随系统"（0.7.8）。 */
    private fun resetScreenBrightness(): Boolean {
        return try {
            runOnUiThread {
                val attrs = window.attributes
                attrs.screenBrightness =
                    WindowManager.LayoutParams.BRIGHTNESS_OVERRIDE_NONE
                window.attributes = attrs
            }
            true
        } catch (e: Exception) {
            false
        }
    }

    /**
     * 当前网络的粗略类型（0.7.3）。
     *
     * 只用于"移动网络下是否自动播放"：Wi-Fi/有线返回 wifi，蜂窝返回 mobile，
     * 无法判断（含 VPN、无网络）返回 unknown，由 Dart 侧按"允许自动播放"处理。
     */
    private fun networkType(): String {
        return try {
            val manager = getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
            val network = manager.activeNetwork ?: return "none"
            val caps = manager.getNetworkCapabilities(network) ?: return "none"
            when {
                caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> "wifi"
                caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> "mobile"
                caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> "ethernet"
                else -> "unknown"
            }
        } catch (e: Exception) {
            "unknown"
        }
    }

    /** 打开本应用的「安装未知应用」设置页。 */
    private fun openInstallSettings(): Boolean {
        return try {
            val intent = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                Intent(android.provider.Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES)
                    .setData(Uri.parse("package:$packageName"))
            } else {
                Intent(android.provider.Settings.ACTION_SECURITY_SETTINGS)
            }
            intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            startActivity(intent)
            true
        } catch (e: Exception) {
            false
        }
    }

    /** 通过 FileProvider 拉起系统安装界面。 */
    private fun installApk(path: String): String {
        val file = File(path)
        if (!file.exists()) return "missing"
        if (!canInstallPackages()) return "no_permission"
        return try {
            val uri = FileProvider.getUriForFile(
                this, "$packageName.updateprovider", file
            )
            val intent = Intent(Intent.ACTION_VIEW).apply {
                setDataAndType(uri, "application/vnd.android.package-archive")
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }
            startActivity(intent)
            "ok"
        } catch (e: Exception) {
            "failed:${e.javaClass.simpleName}"
        }
    }
}
