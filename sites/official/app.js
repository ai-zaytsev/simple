(() => {
  "use strict";

  const manifestUrl = "/releases.json";
  const download = document.querySelector("[data-download]");
  const downloadMeta = document.querySelector("[data-download-meta]");
  const status = document.querySelector("[data-status]");
  const version = document.querySelector("[data-version]");
  const published = document.querySelector("[data-published]");
  const size = document.querySelector("[data-size]");
  const hash = document.querySelector("[data-hash]");
  const copyHash = document.querySelector("[data-copy-hash]");
  const versionList = document.querySelector("[data-version-list]");

  const formatSize = (bytes) => {
    if (!Number.isFinite(bytes) || bytes < 0) return "—";
    return `${(bytes / 1024 / 1024).toFixed(1).replace(".", ",")} МБ`;
  };

  const formatDate = (value) => {
    const date = new Date(value);
    if (Number.isNaN(date.valueOf())) return "—";
    return new Intl.DateTimeFormat("ru-RU", {
      day: "numeric",
      month: "long",
      year: "numeric",
      timeZone: "Europe/Moscow",
    }).format(date);
  };

  const isRelease = (entry) => entry
    && typeof entry.versionName === "string"
    && Number.isInteger(entry.versionCode)
    && typeof entry.publishedAt === "string"
    && Number.isInteger(entry.size)
    && /^[a-f0-9]{64}$/.test(entry.sha256)
    && typeof entry.url === "string"
    && entry.url.startsWith("/download/releases/");

  const escapeText = (value) => {
    const span = document.createElement("span");
    span.textContent = value;
    return span.innerHTML;
  };

  const renderArchive = (releases, latestVersion) => {
    versionList.innerHTML = releases.map((entry) => {
      const latest = entry.versionName === latestVersion
        ? '<span class="version-latest">последняя</span>'
        : "";
      return `
        <article class="version-row">
          <div><span class="version-name">${escapeText(entry.versionName)}</span>${latest}</div>
          <span class="version-date">${escapeText(formatDate(entry.publishedAt))}</span>
          <span class="version-sha" title="SHA-256 ${entry.sha256}">${entry.sha256}</span>
          <a class="version-download" href="${escapeText(entry.url)}" download>Скачать · ${escapeText(formatSize(entry.size))}</a>
        </article>`;
    }).join("");
  };

  const showUnavailable = () => {
    download.removeAttribute("href");
    download.setAttribute("aria-disabled", "true");
    download.classList.add("is-disabled");
    downloadMeta.textContent = "Версия пока не опубликована";
    status.textContent = "Попробуйте обновить страницу позже";
    versionList.innerHTML = '<p class="archive-state">Пока нет опубликованных версий.</p>';
  };

  fetch(manifestUrl, { cache: "no-store", headers: { accept: "application/json" } })
    .then((response) => {
      if (!response.ok) throw new Error(`manifest ${response.status}`);
      return response.json();
    })
    .then((manifest) => {
      if (manifest.schema !== 1 || !Array.isArray(manifest.releases)) {
        throw new Error("unknown manifest schema");
      }
      const releases = manifest.releases.filter(isRelease);
      const latest = releases.find((entry) => entry.versionName === manifest.latest);
      if (!latest || releases.length !== manifest.releases.length) {
        throw new Error("invalid release manifest");
      }

      download.href = "/download/latest.apk";
      download.setAttribute("download", `simple-vpn-${latest.versionName}.apk`);
      download.removeAttribute("aria-disabled");
      download.classList.remove("is-disabled");
      downloadMeta.textContent = `Версия ${latest.versionName} · ${formatSize(latest.size)}`;
      status.textContent = "Нужен Android 7.0 или новее";
      version.textContent = latest.versionName;
      published.textContent = formatDate(latest.publishedAt);
      size.textContent = formatSize(latest.size);
      hash.textContent = latest.sha256;
      copyHash.disabled = false;
      copyHash.dataset.value = latest.sha256;
      renderArchive(releases, latest.versionName);
    })
    .catch(showUnavailable);

  copyHash.addEventListener("click", () => {
    const value = copyHash.dataset.value;
    if (!value || !navigator.clipboard) return;
    navigator.clipboard.writeText(value).then(() => {
      const previous = copyHash.title;
      copyHash.title = "Скопировано";
      setTimeout(() => { copyHash.title = previous; }, 1500);
    });
  });
})();
