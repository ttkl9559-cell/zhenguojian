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
const _b = Drama(id: 'hongguo:2', source: 'hongguo', title: '乙剧', episodes: 3);
const _c = Drama(id: 'hongguo:3', source: 'hongguo', title: '丙剧', episodes: 3);

class _Repo extends FixtureRepository {
  @override
  Future<CatalogPage> recommendations(
    String genre, {
    bool more = false,
    bool force = false,
  }) async => CatalogPage(const [_a, _b, _c]);

  @override
  Future<DramaDetail> detail(Drama drama) async => DramaDetail(drama, [
    Episode({'id': '1', 'title': '第1集', 'currentEpisode': 1}, 1),
    Episode({'id': '2', 'title': '第2集', 'currentEpisode': 2}, 2),
    Episode({'id': '3', 'title': '第3集', 'currentEpisode': 3}, 3),
  ]);

  @override
  Future<PlaybackPlan> resolve(
    Drama drama,
    Episode episode, {
    int quality = 0,
  }) async => PlaybackPlan(
    url: 'https://example.test/${drama.title}${episode.number}.mp4',
    session: 'S-${drama.title}${episode.number}',
  );

  @override
  Future<PlaybackPlan?> preload(
    Drama drama,
    Episode episode, {
    int quality = 0,
    bool online = false,
  }) async => PlaybackPlan(
    url: 'https://example.test/p-${drama.title}${episode.number}.mp4',
    session: 'P-${drama.title}${episode.number}',
  );
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

  testWidgets('横屏视频非全屏：水平滑调进度时页面不应翻页', (tester) async {
    tester.view.devicePixelRatio = 1;
    tester.view.physicalSize = const Size(420, 880);
    addTearDown(tester.view.resetDevicePixelRatio);
    addTearDown(tester.view.resetPhysicalSize);

    final repo = _Repo();
    final platforms = <ScriptedPlayer>[];
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
            playerFactory: () {
              final p = ScriptedPlayer();
              platforms.add(p);
              return Player(platformPlayer: p);
            },
            videoBuilder: (controls) => controls,
          ),
        ),
      ),
    );
    await _pump(tester, 80);

    // 把当前页的播放器报成**横屏**尺寸（1920×1080，比例 > 1.05）。
    for (final p in platforms) {
      p.videoSize(1920, 1080);
    }
    await _pump(tester, 20);

    String current() {
      for (final t in ['甲剧', '乙剧', '丙剧']) {
        if (find.text(t).evaluate().isNotEmpty) return t;
      }
      return '?';
    }

    final first = current();
    debugPrint('起始剧 = $first');

    // 用户在左侧/中部水平滑（带一点竖直分量）调进度。
    await tester.dragFrom(
      const Offset(210, 440),
      const Offset(150, 40),
      touchSlopX: 0,
      touchSlopY: 0,
    );
    await _pump(tester, 40);

    final after = current();
    debugPrint('水平滑动后 = $after');

    expect(after, first, reason: '水平滑动调进度时，页面不应翻到下一部');
    expect(tester.takeException(), isNull);
  });

  testWidgets('横屏视频非全屏：先竖直 30px 再横向拖动，不应翻页', (tester) async {
    tester.view.devicePixelRatio = 1;
    tester.view.physicalSize = const Size(420, 880);
    addTearDown(tester.view.resetDevicePixelRatio);
    addTearDown(tester.view.resetPhysicalSize);

    final repo = _Repo();
    final platforms = <ScriptedPlayer>[];
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
            playerFactory: () {
              final p = ScriptedPlayer();
              platforms.add(p);
              return Player(platformPlayer: p);
            },
            videoBuilder: (controls) => controls,
          ),
        ),
      ),
    );
    await _pump(tester, 80);
    for (final p in platforms) {
      p.videoSize(1920, 1080);
    }
    await _pump(tester, 20);

    String current() {
      for (final t in ['甲剧', '乙剧', '丙剧']) {
        if (find.text(t).evaluate().isNotEmpty) return t;
      }
      return '?';
    }

    final first = current();
    // 真实轨迹：先竖直 30px（未到 PageView 阈值 60），再横向 120px。
    final gesture = await tester.startGesture(const Offset(210, 440));
    await gesture.moveBy(const Offset(0, 30));
    await tester.pump(const Duration(milliseconds: 16));
    await gesture.moveBy(const Offset(120, -8));
    await tester.pump(const Duration(milliseconds: 16));
    await gesture.up();
    await _pump(tester, 40);

    expect(current(), first, reason: '先竖直 30px 再转横向，不应翻页');
  });

  testWidgets('横屏视频非全屏：竖直滑仍然可以翻页（不得回退）', (tester) async {
    tester.view.devicePixelRatio = 1;
    tester.view.physicalSize = const Size(420, 880);
    addTearDown(tester.view.resetDevicePixelRatio);
    addTearDown(tester.view.resetPhysicalSize);

    final repo = _Repo();
    final platforms = <ScriptedPlayer>[];
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
            playerFactory: () {
              final p = ScriptedPlayer();
              platforms.add(p);
              return Player(platformPlayer: p);
            },
            videoBuilder: (controls) => controls,
          ),
        ),
      ),
    );
    await _pump(tester, 80);
    for (final p in platforms) {
      p.videoSize(1920, 1080);
    }
    await _pump(tester, 20);

    String current() {
      for (final t in ['甲剧', '乙剧', '丙剧']) {
        if (find.text(t).evaluate().isNotEmpty) return t;
      }
      return '?';
    }

    final first = current();
    await tester.fling(
      find.byType(PageView),
      const Offset(0, -400),
      1200,
      initialOffset: const Offset(210, 500),
    );
    await _pump(tester, 120);
    final after = current();
    debugPrint('竖直 fling：$first -> $after');
    expect(after, isNot(first), reason: '竖直滑动必须还能翻页');
  });
}
