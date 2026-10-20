/* The setup files and the changelog come from releases.json, the catalog the
   release workflow publishes beside this page. Without that file, or without
   JavaScript, the rows keep their link to the release page. */
(function () {
  var CATALOG = "releases.json";

  function releaseFor(catalog, marker) {
    var tag = catalog[marker];
    if (!tag) return null;
    for (var index = 0; index < catalog.releases.length; index++) {
      if (catalog.releases[index].tag === tag) return catalog.releases[index];
    }
    return null;
  }

  function fileFor(release, row) {
    var os = row.getAttribute("data-os");
    var arch = row.getAttribute("data-arch");
    var format = row.getAttribute("data-format");
    for (var index = 0; index < release.files.length; index++) {
      var file = release.files[index];
      if (file.kind !== "app" && file.kind !== "installer") continue;
      if (file.os === os && file.arch === arch && file.format === format) return file;
    }
    return null;
  }

  function fileSize(bytes) {
    if (!bytes || bytes <= 0) return "";
    var units = ["B", "KB", "MB", "GB"];
    var value = bytes;
    var unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
      value /= 1024;
      unit += 1;
    }
    var shown = value < 10 && unit > 0 ? value.toFixed(1) : String(Math.round(value));
    return shown + " " + units[unit];
  }

  function notesText(markdown) {
    return String(markdown)
      .replace(/\r\n/g, "\n")
      .replace(/\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g, "$1 ($2)")
      .replace(/\*\*(.+?)\*\*/g, "$1")
      .replace(/`([^`]+)`/g, "$1")
      .replace(/^#{1,6}\s+/gm, "")
      .replace(/^\s*[*+-]\s+/gm, "\u2022 ")
      .replace(/\n{3,}/g, "\n\n")
      .trim();
  }

  function fillFiles(release) {
    var rows = document.querySelectorAll(".setup-files li[data-os]");
    for (var index = 0; index < rows.length; index++) {
      var row = rows[index];
      var file = fileFor(release, row);
      if (!file) {
        row.parentNode.removeChild(row);
        continue;
      }
      var link = row.querySelector("a");
      link.href = file.url;
      row.querySelector("code").textContent = file.name;
      var size = fileSize(file.size);
      if (!size) continue;
      var badge = document.createElement("span");
      badge.className = "file-size";
      badge.textContent = size;
      link.appendChild(badge);
    }
    var lists = document.querySelectorAll(".setup-files");
    for (var list = 0; list < lists.length; list++) {
      if (lists[list].querySelector("li")) continue;
      var card = lists[list].closest(".channel");
      if (card) card.hidden = true;
    }
  }

  function fillNotes(marker, release) {
    var details = document.querySelector('details[data-notes="' + marker + '"]');
    if (!details || !release || !release.notes) return;
    details.querySelector("[data-notes-version]").textContent = release.version;
    details.querySelector("[data-notes-body]").textContent = notesText(release.notes);
    details.hidden = false;
  }

  function apply(catalog) {
    var latest = releaseFor(catalog, "latest");
    if (latest) fillFiles(latest);
    fillNotes("latest", latest);
    fillNotes("beta", releaseFor(catalog, "latest_beta"));
  }

  if (!window.fetch) return;
  window.fetch(CATALOG, { cache: "no-cache" })
    .then(function (response) {
      return response.ok ? response.json() : null;
    })
    .then(function (catalog) {
      if (catalog && catalog.releases) apply(catalog);
    })
    .catch(function () {});
})();
