import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

/// 当前网络的粗略类型（0.7.3，`XQ-0107`）。
///
/// 只区分"是不是移动网络"，不追求精确分类：
/// - `wifi`：Wi-Fi 或有线；
/// - `mobile`：蜂窝网络（流量）；
/// - `unknown`：无法判断（非 Android、通道不可用/超时、VPN 等）。
///
/// `unknown` 按"允许自动播放"处理：宁可多播一次，
/// 也不要因为判断失败让用户在 Wi-Fi 下也得点一下（体感是"坏了"）。
enum NetworkKind { wifi, mobile, unknown }

/// 读取当前网络类型。
///
/// 走 `MainActivity` 已有的 `duanju/device` 通道（`networkType`），
/// **不引入任何第三方依赖**。
class NetworkState {
  const NetworkState._();

  static const channel = MethodChannel('duanju/device');

  /// 通道调用超时（0.7.3）。
  ///
  /// **必须有**：在 Flutter 单元测试里，没有 mock 的 MethodChannel 调用
  /// 不会抛异常，而是**永远不返回**（Future 挂起）。若 `_prepare()` 里
  /// 无超时地 await 它，整个视频加载也会被挂死（实测：预加载测试直接超时）。
  /// 真机上是毫秒级返回，这个超时只是保险。
  static const _timeout = Duration(milliseconds: 800);

  static Future<NetworkKind> current() async {
    if (defaultTargetPlatform != TargetPlatform.android) {
      return NetworkKind.unknown;
    }
    try {
      final type = await channel
          .invokeMethod<String>('networkType')
          .timeout(_timeout, onTimeout: () => null);
      return switch (type) {
        'wifi' || 'ethernet' => NetworkKind.wifi,
        'mobile' => NetworkKind.mobile,
        _ => NetworkKind.unknown,
      };
    } on PlatformException {
      return NetworkKind.unknown;
    } on MissingPluginException {
      return NetworkKind.unknown;
    }
  }
}
