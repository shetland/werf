---
title: Директива asLayers и раздельное кэширование инструкций
sidebar: documentation
permalink: documentation/reference/development_and_debug/as_layers.html
author: Alexey Igrychev <alexey.igrychev@flant.com>
summary: |
  <div class="language-yaml highlighter-rouge"><pre class="highlight"><code><span class="s">asLayers</span><span class="pi">:</span> <span class="s">true</span>
  </code></pre>
  </div>
---

Сборка пользовательских стадий `beforeInstall`, `install`, `beforeSetup`, `setup` состоит из последовательного выполнения инструкций, описанных в конфигурации соответствующей стадии. Любое изменение инструкций _стадии_ приводит к пересборке всей стадии и повторному выполнению всех инструкций стадии. Если стадия содержит инструкции которые выполняются значительное время, то первичная разработка может также занимать много времени.

Рассмотрим ситуацию, когда при сборке какой-либо стадии, происходит ошибка при выполнении последней инструкции. В этом случае даже интроспекция стадии не даст непосредственно того состояния системы, которое было перед возникновением ошибки.

Для целей разработки и отладки был реализован механизм _раздельного кэширования инструкций_, который включается в файле конфигурации с помощью директивы _asLayers_.  Во время сборки, при включенном раздельном кэшировании, кэшируется не только сама стадия, но и каждая инструкция. Соответственно пересборка инструкций будет выполняться только в случае если изменился их порядок в стадии, или изменился сам набор инструкций. Директива _asLayers_ может быть указана в файле конфигурации `werf.yaml` в секции конфигурации _образа_ или _артефакта_.

Если в конфигурации _образа_ или _артефакта_ указать `asLayers: true`, то включается режим раздельного кэширования, при котором создается один Docker-слой на каждую команду shell-сборщика или на каждую задачу Ansible-сборщика.

По умолчанию режим раздельного кжширования выключен, т.е. `asLayers: false`. В этом случае работает обычный режим кэширования стадий, когда на все инструкции каждой стадии создается один Docker-слой.

В режиме раздельного кэширования использование параметров [интроспекции]({{site.baseurl}}/documentation/reference/development_and_debug/stage_introspection.html) `--introspect-before-error` и `--introspect-error` позволяет получать окружение (попадать в контейнер) до или после выполнения ошибочной инструкции соответственно.

Изменение режима кэширования регулируется только директивой _asLayes_. Остальные инструкции конфигурации остаются без изменений.

Режим раздельного кэширования предназначен именно **для отладки** и его крайне **не желательно** включать при обычной работе, т.к. в этом режиме генерируется большое количество Docker-слоев и он в целом не эффективен с точки зрения инкрементальной сборки. В этом режиме размер каждой стадии будет больше, как и время, затрачиваемое на сборку стадии.

<div class="videoWrapper">
<iframe width="560" height="315" src="https://www.youtube.com/embed/VEFapPLXxcE" frameborder="0" allow="encrypted-media" allowfullscreen></iframe>
</div>
