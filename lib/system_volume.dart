import 'dart:io' show Platform;

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

/// 系统媒体音量（0.7.5，`XQ-0109`）。
///
/// 播放器右滑/上下滑调的是**系统媒体音量**，不是播放器自身的音量；
/// 应用内音量保持 100%，避免"两层音量"叠加。
///
/// 走 `MainActivity` 的 `duanju/device` 通道（`systemVolume` / `setSystemVolume`），
/// 不引入第三方依赖。
class SystemVolume {
  const SystemVolume._();

  static const channel = MethodChannel('duanju/device');

  /// 通道超时：理由同 `NetworkState`（测试环境未 mock 的通道会永久挂起）。
  static const _timeout = Duration(milliseconds: 800);

  /// 单元测试环境直接短路（0.7.6）。
  ///
  /// **为什么必须短路**：`Future.timeout` 会创建一个 800ms 的 `Timer`；
  /// 若在 `initState`（页面创建）里调用，测试结束时会报
  /// 「A Timer is still pending even after the widget tree was disposed」，
  /// 让本来与音量无关的用例失败（实测踩到）。
  static bool get _testing => Platform.environment['FLUTTER_TEST'] == 'true';

  /// 读取当前档位与最大档位；不可用时返回 `null`。
  static Future<({int current, int max})?> read() async {
    if (_testing) return null;
    if (defaultTargetPlatform != TargetPlatform.android) return null;
    try {
      final data = await channel
          .invokeMapMethod<String, dynamic>('systemVolume')
          .timeout(_timeout, onTimeout: () => null);
      if (data == null) return null;
      final current = (data['current'] as num?)?.toInt() ?? 0;
      final max = (data['max'] as num?)?.toInt() ?? 0;
      if (max <= 0) return null;
      return (current: current, max: max);
    } on PlatformException {
      return null;
    } on MissingPluginException {
      return null;
    }
  }

  /// 设置系统媒体音量档位；超时/不可用视为成功（不阻塞手势）。
  static Future<void> write(int index) async {
    if (_testing) return;
    if (defaultTargetPlatform != TargetPlatform.android) return;
    try {
      await channel
          .invokeMethod<bool>('setSystemVolume', {'index': index})
          .timeout(_timeout, onTimeout: () => null);
    } on PlatformException {
      // 忽略：个别设备无权限或被策略限制，不影响观看。
    } on MissingPluginException {
      // 忽略：桌面端/测试环境没有该通道。
    }
  }
}
