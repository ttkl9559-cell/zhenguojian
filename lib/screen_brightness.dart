import 'dart:io' show Platform;

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

/// 窗口亮度（0.7.8，`XQ-0114`）。
///
/// 用**本 App 窗口亮度**覆盖系统亮度：
/// - 系统亮度调得再暗，App 内也能把画面拉亮（上限是屏幕硬件最亮）；
/// - 不需要「修改系统设置」特殊权限；
/// - 退出播放页 / 退出全屏时恢复"跟随系统"，不会把手机亮度改掉。
///
/// 走 `MainActivity` 的 `duanju/device` 通道，不引入第三方依赖。
class ScreenBrightness {
  const ScreenBrightness._();

  static const channel = MethodChannel('duanju/device');

  /// 通道超时：理由同 `SystemVolume`（测试环境未 mock 的通道会永久挂起）。
  static const _timeout = Duration(milliseconds: 800);

  /// 单元测试环境直接短路，避免 `Future.timeout` 留下未清理的计时器。
  static bool get _testing => Platform.environment['FLUTTER_TEST'] == 'true';

  /// 读取系统当前亮度比例（0~1）；不可用时返回 `null`。
  static Future<double?> read() async {
    if (_testing) return null;
    if (defaultTargetPlatform != TargetPlatform.android) return null;
    try {
      final value = await channel
          .invokeMethod<double>('screenBrightness')
          .timeout(_timeout, onTimeout: () => null);
      if (value == null) return null;
      return value.clamp(0.0, 1.0);
    } on PlatformException {
      return null;
    } on MissingPluginException {
      return null;
    }
  }

  /// 设置本窗口亮度（0~1）。
  static Future<void> set(double ratio) async {
    if (_testing) return;
    if (defaultTargetPlatform != TargetPlatform.android) return;
    try {
      await channel
          .invokeMethod<bool>('setScreenBrightness', {
            'value': ratio.clamp(0.0, 1.0),
          })
          .timeout(_timeout, onTimeout: () => null);
    } on PlatformException {
      // 忽略：个别设备策略限制，不影响观看。
    } on MissingPluginException {
      // 忽略：桌面端/测试环境没有该通道。
    }
  }

  /// 恢复窗口亮度为"跟随系统"。
  static Future<void> reset() async {
    if (_testing) return;
    if (defaultTargetPlatform != TargetPlatform.android) return;
    try {
      await channel
          .invokeMethod<bool>('resetScreenBrightness')
          .timeout(_timeout, onTimeout: () => null);
    } on PlatformException {
      // 忽略。
    } on MissingPluginException {
      // 忽略。
    }
  }
}
