// navdropdown.js — keeps a header dropdown on screen, whatever its width or position.
//
// The CSS anchors a dropdown `left: 0` to its nav item, which is correct while every menu is narrow
// and sits well left of the viewport edge. Two things broke that. Folding the guides together made
// two menus wide enough to run past the right edge from the last nav position, and at 768px and
// below `.main-nav` becomes `overflow-x: auto`, which CLIPS an absolutely positioned child: the
// dropdown was not merely off screen there, it was cut off by its own scrolling ancestor.
//
// Both go away by taking the dropdown out of the flow on open. It is positioned `fixed` against the
// viewport and clamped into it, so no ancestor can clip it and no nav position can push it out. The
// CSS keeps its `left: 0` rule as the no-JS baseline, which is right for the common wide case.
(function () {
  "use strict";

  var MARGIN = 8;
  var open = null;

  // Below the collapse breakpoint the menu is a stacked list with its submenus already inline, so
  // positioning anything would fight the stylesheet. One check, in both entry points.
  function collapsed() {
    return window.matchMedia("(max-width: 768px)").matches;
  }

  function place(item) {
    var dd = item.querySelector(".nav-dropdown");
    if (!dd || collapsed()) return;

    // The dropdown is `display: none` until CSS :hover shows it, and pointerenter can fire while it
    // is still hidden. A hidden element measures ZERO, so reading offsetWidth here without forcing
    // display first computed the clamp against a width of 0 and positioned the menu as though it
    // took no space, which looked exactly like no clamp at all. Show it, then measure.
    dd.style.display = "block";
    dd.style.position = "fixed";
    dd.style.left = "0px";
    dd.style.top = "0px";
    dd.style.maxHeight = "";
    dd.style.width = "";

    var r = item.getBoundingClientRect();
    var w = dd.offsetWidth;
    var vw = document.documentElement.clientWidth;
    var vh = document.documentElement.clientHeight;

    // Too wide to fit at all (a narrow window): span the viewport and let the column rules collapse.
    if (w > vw - MARGIN * 2) {
      dd.style.left = MARGIN + "px";
      dd.style.width = vw - MARGIN * 2 + "px";
    } else {
      dd.style.left = Math.min(Math.max(MARGIN, r.left), vw - w - MARGIN) + "px";
    }

    dd.style.top = r.bottom + "px";
    dd.style.maxHeight = vh - r.bottom - MARGIN + "px";
    open = item;
  }

  function clear(item) {
    var dd = item.querySelector(".nav-dropdown");
    if (!dd) return;
    // Always runs, even collapsed: a resize down from desktop must strip inline styles set earlier.
    dd.style.display = "";
    dd.style.position = "";
    dd.style.left = "";
    dd.style.top = "";
    dd.style.width = "";
    dd.style.maxHeight = "";
    if (open === item) open = null;
  }

  function run() {
    var items = document.querySelectorAll(".main-nav .nav-item.has-children");
    items.forEach(function (item) {
      item.addEventListener("pointerenter", function () { place(item); });
      item.addEventListener("pointerleave", function () { clear(item); });
      // Keyboard: the dropdown holds real links, so focus moving into it must not reposition or drop it.
      item.addEventListener("focusin", function () { place(item); });
      item.addEventListener("focusout", function (e) {
        if (!item.contains(e.relatedTarget)) clear(item);
      });
    });

    // A fixed dropdown does not travel with the page, so re-place it rather than let it detach.
    window.addEventListener("scroll", function () { if (open) place(open); }, { passive: true });
    window.addEventListener("resize", function () {
      if (!open) return;
      // Crossing the breakpoint with a menu open would otherwise strand desktop inline styles on
      // the collapsed layout, which is how a submenu ends up floating over the stacked list.
      if (collapsed()) clear(open); else place(open);
    });

    toggle("nav-toggle", document.querySelector(".site-header"), "nav-open");
    toggle("sidebar-toggle", document.querySelector(".sidebar"), "is-open");
  }

  // toggle wires a button to a class on its container and keeps aria-expanded honest, since the
  // button is the only affordance a screen reader has for a menu that is visually collapsed.
  function toggle(buttonId, container, cls) {
    var btn = document.getElementById(buttonId);
    if (!btn || !container) return;
    btn.addEventListener("click", function () {
      var nowOpen = container.classList.toggle(cls);
      btn.setAttribute("aria-expanded", nowOpen ? "true" : "false");
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", run);
  } else {
    run();
  }
})();
