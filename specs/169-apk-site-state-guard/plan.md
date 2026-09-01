# Plan: Устойчивое чтение state APK-сайта

1. Заменить pipe чтением state в файл в inventory и reconcile.
2. Расширить regression test обоими инвариантами.
3. Зафиксировать live recovery в BO/spec memory.
4. После merge выполнить dry-run и получить state-known/no-change verdict.
