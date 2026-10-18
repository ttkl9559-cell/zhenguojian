import 'package:duanju_app/local_store.dart';
import 'package:duanju_app/models.dart';
import 'package:duanju_app/vertical_feed_screen.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:media_kit/media_kit.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'fixtures.dart';
import 'player_fixtures.dart';

const _a = Drama(id: 'hongguo:1', source: 'hongguo', title: '甲剧', episodes: 3);

class _Repo extends FixtureRepository {
  final resolves = <String>[];

  @override
  Future<CatalogPage> recommendations(
    String genre, {
    bool more = false,
    bool force = false,
  }) async => CatalogPage(const [_a]);

  @override
  Future<DramaDetail> detail(Drama drama) async => DramaDetail(drama, [
    Episode({'id': '1', 'title': '第1集', 'currentEpisode': 1}, 1),
    Episode({'id': '2', 'title': '第2集', 'currentEpisode': 2}, 2),
  ]);

  @override
  Future<PlaybackPlan> resolve(
    Drama drama,
    Episode episode, {
    int quality = 0,
  }) async {
    resolves.add('${episode.number}');
    return PlaybackPlan(
      url: 'https://example.test/r${episode.number}.mp4',
      session: 'R${episode.number}-${resolves.length}',
    );
  }
}

/// 模拟 0.6.11 要修的那个真实故障：
///
/// **起播一次后立刻卡住**——播放器先报告 `playing = true`（于是页面清掉
/// `_awaitingFirstFrame`），随后掉回 `playing = false` 且位置不再前进。
///
/// 修复前：兜底一要求 `_awaitingFirstFrame`（已被清），兜底二要求
/// `playing == true`（已为 false），**两条同时失效 → 永久纯黑无文字**。
///
/// ⚠️ 时序必须与真机一致：`open()` 返回时**还不能**报告 `playing = true`。
/// 真机上"地址交出"与"真正起播"是两次独立的异步事件；
/// 若在这里同步置 `playing = true`，`_prepare()` 尾部那次 `setState`
/// 会把它覆盖回 `true`，于是测的就不是本条兜底了（本轮踩过）。
class _StallsAfterPlay extends ScriptedPlayer {
  @override
  Future<void> open(Playable playable, {bool play = true}) async {
    final media = playable as Media;
    opened.add(media);
    played.add(play);
    // 地址已交出，但**尚未起播**（真实弱网/慢盘下的常见状态）。
    state = state.copyWith(
      position: Duration.zero,
      duration: const Duration(minutes: 2),
      playing: false,
      completed: false,
      buffering: false,
      width: 320,
      height: 180,
    );
    positionController.add(Duration.zero);
    durationController.add(state.duration);
  }

  /// 稍后真正起播（异步事件），这一步会清掉页面的 `_awaitingFirstFrame`。
  void startPlaying() {
    state = state.copyWith(playing: true);
    playingController.add(true);
  }

  /// 模拟卡住：`playing` 掉回 false，位置停在原地。
  ///
  /// 注意这里**不是** `pause()`——`pause()` 是"用户主动暂停"的语义，
  /// 会走 `_togglePlayback` 并置 `_userPaused`。这里模拟的是**播放中断**，
  /// 直接改状态并推送 playing 流，等价于底层解码/网络自己停了。
  void stall() {
    state = state.copyWith(playing: false);
    playingController.add(false);
  }
}

Future<void> _pump(WidgetTester tester, int frames) async {
  for (var i = 0; i < frames; i++) {
    await tester.pump(const Duration(milliseconds: 20));
  }
}

void main() {
  late LocalStore store;

  setUp(() async {
    SharedPreferences.setMockInitialValues(const {});
    store = LocalStore(await SharedPreferences.getInstance());
  });

  tearDown(() => store.dispose());

  Future<List<_StallsAfterPlay>> mount(
    WidgetTester tester,
    _Repo repo,
  ) async {
    tester.view.devicePixelRatio = 1;
    tester.view.physicalSize = const Size(420, 880);
    addTearDown(tester.view.resetDevicePixelRatio);
    addTearDown(tester.view.resetPhysicalSize);
    final platforms = <_StallsAfterPlay>[];
    await tester.pumpWidget(
      MaterialApp(
        theme: ThemeData.dark(),
        home: Scaffold(
          body: VerticalFeedScreen(
            key: const ValueKey('feed'),
            repository: repo,
            store: store,
            embedded: true,
            active: true,
            singleDrama: _a,
            playerFactory: () {
              final p = _StallsAfterPlay();
              platforms.add(p);
              return Player(platformPlayer: p);
            },
            videoBuilder: (controls) => controls,
          ),
        ),
      ),
    );
    await _pump(tester, 60);
    return platforms;
  }

  testWidgets(
    '0.6.11：起播后又卡住时**自动恢复**（修复前会永久纯黑无文字）',
    (tester) async {
      final repo = _Repo();
      final platforms = await mount(tester, repo);

      expect(platforms, isNotEmpty);
      expect(platforms.last.opened, isNotEmpty, reason: '地址应已交给播放器');
      // 关键前提：此时**尚未起播**（模拟真实异步时序）。
      expect(platforms.last.state.playing, isFalse);

      // 真正起播 → 这一步会清掉页面的 `_awaitingFirstFrame`，
      // 使兜底一失效，从而只可能由兜底三兜住。
      platforms.last.startPlaying();
      await _pump(tester, 5);
      expect(platforms.last.state.playing, isTrue, reason: '应已起播');

      // 起播后立刻卡住：playing 掉回 false，位置不再前进。
      platforms.last.stall();
      await _pump(tester, 5);
      expect(platforms.last.state.playing, isFalse);

      final before = repo.resolves.length;
      final openBefore = platforms.last.opened.length;

      // 推进 **12 秒**：大于兜底三的 10 秒，小于兜底一的 15 秒。
      // 这个区间能唯一区分"兜底三生效"与"其实是兜底一在救"。
      await _pump(tester, 600);

      final after = repo.resolves.length;
      final openAfter = platforms.last.opened.length;
      expect(
        after,
        greaterThan(before),
        reason: '起播后卡住（playing=false、_awaitingFirstFrame 已清）时，'
            '必须由第三条兜底在约 10 秒内自动恢复，'
            '否则就是永久纯黑无文字。'
            '（resolves $before->$after, opened $openBefore->$openAfter）',
      );
    },
  );

  testWidgets(
    '0.6.11：**用户主动暂停不会被当成故障**自动播回去',
    (tester) async {
      final repo = _Repo();
      final platforms = await mount(tester, repo);
      // 先让它真正起播，再模拟用户双击暂停。
      platforms.last.startPlaying();
      await _pump(tester, 5);
      expect(platforms.last.state.playing, isTrue);

      // 双击画面 = 用户暂停（走 `_togglePlayback`，置 `_userPaused`）。
      final target = find.byType(GestureDetector).first;
      await tester.tap(target);
      await tester.pump(const Duration(milliseconds: 40));
      await tester.tap(target);
      await _pump(tester, 10);

      // 前提：确实进入暂停态。
      expect(
        platforms.last.state.playing,
        isFalse,
        reason: '双击后应处于暂停态',
      );

      final before = repo.resolves.length;
      // 推进 20 秒（远超兜底三的 10 秒）。
      await _pump(tester, 1000);
      final after = repo.resolves.length;

      expect(
        after,
        equals(before),
        reason: '用户主动暂停必须保持暂停，不能被自动恢复逻辑播回去',
      );
      expect(platforms.last.state.playing, isFalse, reason: '应保持暂停');
    },
  );
}
