import 'dart:convert';

import 'package:duanju_app/app_bottom_navigation.dart';
import 'package:duanju_app/app_theme.dart';
import 'package:duanju_app/home_screen.dart';
import 'package:duanju_app/local_store.dart';
import 'package:duanju_app/main.dart';
import 'package:duanju_app/settings_screen.dart';
import 'package:duanju_app/vertical_feed_screen.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:media_kit/media_kit.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'fixtures.dart';
import 'player_fixtures.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  Future<LocalStore> localStore() async {
    SharedPreferences.setMockInitialValues({});
    return LocalStore(await SharedPreferences.getInstance());
  }

  void phone(WidgetTester tester, [double width = 390]) {
    tester.view.physicalSize = Size(width, 844);
    tester.view.devicePixelRatio = 1;
    tester.view.padding = const FakeViewPadding(top: 24, bottom: 24);
    addTearDown(tester.view.reset);
  }

  test(
    'theme survives restart and backup, with compatible old backups',
    () async {
      final store = await localStore();
      expect(store.themeMode, 'system');
      await store.setThemeMode('light');
      final restored = LocalStore(store.preferences);
      expect(restored.themeMode, 'light');
      final backup = await store.exportBackup();
      await store.setThemeMode('system');
      await store.importBackup(backup);
      expect(store.themeMode, 'light');
      final old = jsonDecode(backup) as Map<String, dynamic>;
      old.remove('themeMode');
      await store.setThemeMode('system');
      await store.importBackup(jsonEncode(old));
      expect(store.themeMode, 'system');
      old['themeMode'] = 'invalid';
      await expectLater(
        store.importBackup(jsonEncode(old)),
        throwsFormatException,
      );
      expect(store.themeMode, 'system');
      store.dispose();
      restored.dispose();
    },
  );

  testWidgets(
    'default theme follows the system from bootstrap through browsing',
    (tester) async {
      phone(tester);
      addTearDown(tester.platformDispatcher.clearPlatformBrightnessTestValue);
      final repository = FixtureRepository();
      tester.platformDispatcher.platformBrightnessTestValue = Brightness.light;
      await tester.pumpWidget(DuanjuApp(repository: repository));
      await tester.pump();
      expect(
        tester.widget<MaterialApp>(find.byType(MaterialApp)).themeMode,
        ThemeMode.system,
      );
      final store = await localStore();
      await tester.pumpWidget(DuanjuApp(repository: repository, store: store));
      await tester.pumpAndSettle();
      for (final brightness in [Brightness.dark, Brightness.light]) {
        tester.platformDispatcher.platformBrightnessTestValue = brightness;
        await tester.pumpAndSettle();
        expect(
          Theme.of(tester.element(find.byType(HomeScreen))).brightness,
          brightness,
        );
      }
      await store.setThemeMode('dark');
      await tester.pumpAndSettle();
      expect(
        Theme.of(tester.element(find.byType(HomeScreen))).brightness,
        Brightness.dark,
      );
      expect(LocalStore(store.preferences).themeMode, 'dark');
      await tester.pumpWidget(const SizedBox.shrink());
      store.dispose();
    },
  );

  testWidgets('settings changes open routes without losing the active tab', (
    tester,
  ) async {
    phone(tester);
    final store = await localStore();
    final repository = FixtureRepository();
    await tester.pumpWidget(DuanjuApp(repository: repository, store: store));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('bottom-nav-2')));
    await tester.pumpAndSettle();
    expect(find.text('我的追剧'), findsOneWidget);
    await tester.tap(find.byTooltip('更多'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('设置与备份'));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey('theme-setting')));
    await tester.pumpAndSettle();
    await tester.tap(find.text('浅色'));
    await tester.pumpAndSettle();
    expect(store.themeMode, 'light');
    expect(
      Theme.of(tester.element(find.byType(SettingsScreen))).brightness,
      Brightness.light,
    );
    expect(
      SystemChrome.latestStyle?.systemNavigationBarIconBrightness,
      Brightness.dark,
    );
    expect(
      SystemChrome.latestStyle?.systemNavigationBarContrastEnforced,
      isFalse,
    );
    await tester.tap(find.byType(BackButton).last);
    await tester.pumpAndSettle();
    expect(find.text('我的追剧'), findsOneWidget);
    expect(
      tester
          .widget<AppBottomNavigation>(find.byType(AppBottomNavigation))
          .selectedIndex,
      2,
    );
    final requests = repository.requests.length;
    await store.setThemeMode('system');
    tester.platformDispatcher.platformBrightnessTestValue = Brightness.dark;
    await tester.pumpAndSettle();
    expect(
      Theme.of(tester.element(find.byType(HomeScreen))).brightness,
      Brightness.dark,
    );
    tester.platformDispatcher.platformBrightnessTestValue = Brightness.light;
    await tester.pumpAndSettle();
    expect(
      Theme.of(tester.element(find.byType(HomeScreen))).brightness,
      Brightness.light,
    );
    expect(repository.requests.length, requests);
    expect(find.text('我的追剧'), findsOneWidget);
    tester.platformDispatcher.clearPlatformBrightnessTestValue();
    await tester.pumpWidget(const SizedBox.shrink());
    store.dispose();
  });

  for (final count in [3, 4]) {
    testWidgets(
      '$count navigation destinations support large text and keyboard input',
      (tester) async {
        phone(tester, 320);
        final semantics = tester.ensureSemantics();
        var selected = 0;
        await tester.pumpWidget(
          MaterialApp(
            theme: AppTheme.light,
            home: StatefulBuilder(
              builder: (context, setState) => MediaQuery(
                data: MediaQuery.of(
                  context,
                ).copyWith(textScaler: TextScaler.linear(2)),
                child: Scaffold(
                  bottomNavigationBar: AppBottomNavigation(
                    selectedIndex: selected,
                    onDestinationSelected: (value) =>
                        setState(() => selected = value),
                    destinations: [
                      const NavigationDestination(
                        icon: Icon(Icons.explore_outlined),
                        label: '发现',
                      ),
                      const NavigationDestination(
                        icon: Icon(Icons.bookmark_border),
                        label: '追剧',
                      ),
                      const NavigationDestination(
                        icon: Icon(Icons.history),
                        label: '最近观看',
                      ),
                      if (count == 4)
                        const NavigationDestination(
                          icon: Icon(Icons.download_outlined),
                          label: '下载',
                        ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        );
        await tester.pumpAndSettle();
        for (var i = 0; i < count; i++) {
          final size = tester.getSize(find.byKey(ValueKey('bottom-nav-$i')));
          expect(size.width, greaterThanOrEqualTo(48));
          expect(size.height, greaterThanOrEqualTo(48));
        }
        await tester.tap(find.byKey(const ValueKey('bottom-nav-1')));
        await tester.pumpAndSettle();
        expect(selected, 1);
        for (var i = 0; i < count; i++) {
          await tester.sendKeyEvent(LogicalKeyboardKey.tab);
          await tester.sendKeyEvent(LogicalKeyboardKey.enter);
          await tester.pumpAndSettle();
          expect(selected, i);
        }
        expect(find.text('最近观看'), findsOneWidget);
        semantics.dispose();
        expect(tester.takeException(), isNull);
      },
    );
  }

  testWidgets('player keeps a dark surface and restores light theme on return', (
    tester,
  ) async {
    phone(tester);
    final store = await localStore();
    await store.setThemeMode('light');
    final repository = RouteRepository();
    final platform = ScriptedPlayer();
    await tester.pumpWidget(DuanjuApp(repository: repository, store: store));
    await tester.pumpAndSettle();
    final detail = await repository.detail(FixtureRepository.free);
    final navigator = Navigator.of(tester.element(find.byType(HomeScreen)));
    navigator.push(
      MaterialPageRoute<void>(
        // 2026-10-17：由老播放器 `PlayerScreen` 迁移到新播放器
        // `VerticalFeedScreen`（老播放器已删除，XF-0003 / WT-0031）。
        //
        // 迁移时发现两处与老播放器的**真实差异**（不是测试写法问题）：
        // 1. 老播放器用 `Theme(data: AppTheme.dark)` 包住整页，故播放页
        //    `Theme.of(...).brightness == dark` 成立；
        //    **新播放器竖屏层没有包 `Theme`**，只用 `ColoredBox(Colors.black)`
        //    作深色底（深色主题只在全屏层与弹窗里显式给出）。
        //    → 因此断言改为「深色底」而非「深色主题」。
        // 2. 老播放器用 `AnnotatedRegion<SystemUiOverlayStyle>` 强制深色系统栏；
        //    新播放器无此实现（WT-0031 缺失项 3，用户决定不保留），
        //    → 系统栏相关断言已移除。
        builder: (_) => VerticalFeedScreen(
          repository: repository,
          store: store,
          singleDrama: detail.drama,
          initialEpisodeIndex: 0,
          playerFactory: () => Player(platformPlayer: platform),
          videoBuilder: (controls) =>
              SizedBox(key: const ValueKey('video-theme'), child: controls),
        ),
      ),
    );
    for (var i = 0; i < 15; i++) {
      await tester.pump(const Duration(milliseconds: 40));
    }
    // 播放页必须是深色底（黑），否则浅色主题下会出现白底闪屏。
    final surface = tester.widget<ColoredBox>(
      find
          .ancestor(
            of: find.byKey(const ValueKey('video-theme')),
            matching: find.byType(ColoredBox),
          )
          .first,
    );
    expect(surface.color, Colors.black);
    navigator.pop();
    await tester.pumpAndSettle();
    expect(
      Theme.of(tester.element(find.byType(HomeScreen))).brightness,
      Brightness.light,
    );
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox.shrink());
    store.dispose();
  });
}
