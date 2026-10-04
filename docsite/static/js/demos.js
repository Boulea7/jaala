// Live examples. Each <figure class="demo"> the build rendered carries its spec in data-spec. This
// adds an Edit button that swaps the example's source for a textarea and a Run button, and runs the
// edited rules and goal on jaala compiled to wasm (docsite/wasm), through the same demo.Run the build
// used. The engine loads on the first Run on a page, so a reader who only reads downloads nothing.
// Everything the engine returns is written with textContent, never as HTML.
(function () {
  "use strict";

  var engine = null; // a promise of jaalaRun, once the first Run asks for it

  function loadScript(src) {
    return new Promise(function (resolve, reject) {
      var s = document.createElement("script");
      s.src = src;
      s.onload = resolve;
      s.onerror = function () { reject(new Error("couldn't load " + src)); };
      document.head.appendChild(s);
    });
  }

  function loadEngine() {
    if (engine) return engine;
    var body = document.body;
    engine = loadScript(body.dataset.jaalaGlue)
      .then(function () {
        var go = new Go();
        var url = body.dataset.jaalaWasm;
        var instantiate = WebAssembly.instantiateStreaming
          ? WebAssembly.instantiateStreaming(fetch(url), go.importObject)
          : fetch(url).then(function (r) { return r.arrayBuffer(); })
              .then(function (b) { return WebAssembly.instantiate(b, go.importObject); });
        return instantiate.then(function (result) {
          go.run(result.instance); // main registers jaalaRun and then waits for calls
          if (typeof window.jaalaRun !== "function") throw new Error("the engine started without jaalaRun");
          return window.jaalaRun;
        });
      })
      .catch(function (err) {
        engine = null; // let the next Run try again
        throw err;
      });
    return engine;
  }

  function el(tag, cls, text) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }

  // renderAnswer draws a result the way the build does (docsite/demos.go renderDemo).
  function renderAnswer(answer, result, showCites) {
    answer.replaceChildren();
    if (result.error) {
      answer.appendChild(el("p", "demo-error", result.error));
      return;
    }
    if (!result.rows || result.rows.length === 0) {
      answer.appendChild(el("p", "demo-empty", "No rows."));
      return;
    }
    var table = el("table");
    var head = el("tr");
    result.columns.forEach(function (c) { head.appendChild(el("th", "", c)); });
    if (showCites) head.appendChild(el("th", "demo-cites", "cites"));
    table.appendChild(el("thead")).appendChild(head);
    var body = table.appendChild(el("tbody"));
    result.rows.forEach(function (row, i) {
      var tr = el("tr");
      row.forEach(function (v) { tr.appendChild(el("td", "", v)); });
      if (showCites) tr.appendChild(el("td", "demo-cites", (result.cites[i] || []).join(", ")));
      body.appendChild(tr);
    });
    answer.appendChild(table);
  }

  function enhance(fig) {
    var spec;
    try {
      spec = JSON.parse(fig.dataset.spec);
    } catch (e) {
      return; // no spec, nothing to edit
    }
    var original = ((spec.program || "").trim() + "\n" + (spec.query || "").trim()).trim();
    var answer = fig.querySelector(".demo-answer");
    if (!answer) return;
    var built = answer.cloneNode(true); // the build's answer, which Reset puts back

    var bar = el("div", "demo-bar");
    var edit = el("button", "demo-button", "Edit");
    edit.type = "button";
    bar.appendChild(edit);
    fig.insertBefore(bar, answer);

    var editor = null;
    edit.addEventListener("click", function () {
      if (editor) return;
      var source = fig.querySelector(".code-block") || fig.querySelector(".demo-source");
      editor = el("div", "demo-editor");
      var text = el("textarea", "demo-text");
      text.value = original;
      text.spellcheck = false;
      text.setAttribute("aria-label", "Rules and goal");
      text.rows = Math.max(3, original.split("\n").length + 1);
      editor.appendChild(text);
      var facts = null;
      if (spec.facts) {
        facts = el("textarea", "demo-text demo-facts");
        facts.value = spec.facts.trim();
        facts.spellcheck = false;
        facts.setAttribute("aria-label", "Facts");
        facts.rows = Math.max(2, facts.value.split("\n").length + 1);
        editor.appendChild(el("p", "demo-label", spec.fixture ? "More facts, added to the " + spec.fixture + " fixture:" : "Facts:"));
        editor.appendChild(facts);
      }
      if (source) source.hidden = true;
      fig.insertBefore(editor, bar);

      bar.replaceChildren();
      var run = el("button", "demo-button demo-run", "Run");
      run.type = "button";
      var reset = el("button", "demo-button", "Reset");
      reset.type = "button";
      var status = el("span", "demo-status");
      status.setAttribute("aria-live", "polite");
      bar.appendChild(run);
      bar.appendChild(reset);
      bar.appendChild(status);

      run.addEventListener("click", function () {
        run.disabled = true;
        status.textContent = engine ? "Running…" : "Loading jaala…";
        loadEngine()
          .then(function (jaalaRun) {
            var edited = {
              fixture: spec.fixture || "",
              facts: facts ? facts.value : "",
              program: text.value,
              query: "",
              bind: spec.bind || null,
              cites: !!spec.cites
            };
            renderAnswer(answer, JSON.parse(jaalaRun(JSON.stringify(edited))), !!spec.cites);
            status.textContent = "";
          })
          .catch(function (err) {
            status.textContent = "Couldn't run it here: " + err.message;
          })
          .then(function () { run.disabled = false; });
      });
      reset.addEventListener("click", function () {
        text.value = original;
        if (facts) facts.value = spec.facts.trim();
        answer.replaceChildren.apply(answer, Array.from(built.cloneNode(true).childNodes));
        status.textContent = "";
      });
      text.focus();
    });
  }

  function init() {
    document.querySelectorAll("figure.demo[data-spec]").forEach(enhance);
  }
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", init);
  } else {
    init();
  }
})();
