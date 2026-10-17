// certificates.js: claim, issue again and share a certificate.
import { api, errorMessage } from "../api.js";
import { h } from "../ui/dom.js";
import { openDialog } from "../ui/dialog.js";
import { toast, toastNext } from "../ui/toast.js";

const grid = document.querySelector(".cert-grid");
const displayName = grid?.dataset.displayName || "";

// The certificate fonts draw Latin, Greek and Cyrillic letters; the server
// checks every glyph, this only marks what certainly cannot print.
const UNPRINTABLE = /[^\p{Script=Latin}\p{Script=Greek}\p{Script=Cyrillic}\p{Script=Common}\p{Script=Inherited}]|\p{Extended_Pictographic}/u;

/** The preview of the name: letters that cannot print get a red wash and a hidden label. */
function previewNodes(name) {
  const out = [];
  let plain = "";
  let bad = 0;
  for (const ch of name) {
    if (UNPRINTABLE.test(ch)) {
      if (plain) out.push(plain);
      plain = "";
      bad++;
      out.push(h("mark", { class: "no-glyph" }, ch, h("span", { class: "visually-hidden" }, " (cannot print)")));
    } else {
      plain += ch;
    }
  }
  if (plain) out.push(plain);
  return { nodes: out.length ? out : [" "], bad };
}

for (const card of document.querySelectorAll("[data-kind]")) {
  card.querySelector("[data-claim]")?.addEventListener("click", (event) => claim(card, event.currentTarget.dataset.claim === "again"));
  card.querySelector("[data-copy-link]")?.addEventListener("click", async () => {
    const url = new URL(card.dataset.url, location.origin).href;
    try {
      await navigator.clipboard.writeText(url);
      toast("Verify link copied.");
    } catch {
      toast(`The browser did not allow the copy. The link is ${url}`, { kind: "info" });
    }
  });
}

async function claim(card, again) {
  const kind = card.dataset.kind;
  const label = card.dataset.label;
  const input = h("input", { class: "input", id: "claim-name", name: "name", maxlength: 70, value: card.dataset.name || displayName, autocomplete: "name", "aria-describedby": "claim-name-help" });
  const error = h("p", { class: "field-error", id: "claim-name-error", hidden: true });
  const preview = h("dd", {});
  const previewNote = h("p", { class: "field-error", hidden: true });
  const showPreview = () => {
    const { nodes, bad } = previewNodes(input.value.trim());
    preview.replaceChildren(...nodes);
    previewNote.hidden = bad === 0;
    previewNote.textContent = bad ? "The marked letters cannot print. Use Latin, Greek or Cyrillic letters." : "";
  };
  showPreview();
  const stats = h("input", { type: "checkbox", id: "claim-stats", name: "showStats", checked: card.dataset.showStats === "true", "aria-describedby": "claim-stats-help" });
  input.addEventListener("input", showPreview);
  const body = [
    again
      ? h("p", { class: "callout callout-caution" }, h("b", {}, "CAUTION:"), " The new certificate gets a new ID. The old ID shows as revoked and points to the new one.")
      : null,
    h("div", { class: "field" },
      h("label", { for: "claim-name" }, "Name to print"),
      input,
      h("p", { class: "help", id: "claim-name-help" }, "1 to 70 characters. Latin, Greek and Cyrillic letters print."),
      error),
    h("dl", { class: "titleblock name-preview", "aria-label": "The name as it prints" }, h("div", {}, h("dt", {}, "Name"), preview)),
    previewNote,
    h("div", {},
      h("label", { class: "check", for: "claim-stats" }, stats, h("span", {}, "Show my activity on the certificate and the verify page")),
      h("p", { class: "help", id: "claim-stats-help" }, "Active time, runs, and the hints, explanations, walkthroughs and solutions you revealed.")),
    h("p", { class: "form-status", role: "status" }),
  ];
  let issued = null;
  const v = await openDialog({
    title: again ? `Issue the ${label} again` : `Claim the ${label}`,
    body,
    buttons: [
      { label: "Issue the certificate", value: "ok", kind: "primary" },
      { label: "Cancel", value: "" },
    ],
    // The dialog stays open until the server accepts the name.
    check: (form) => {
      if (issued) return "";
      const button = form.querySelector('button[value="ok"]');
      if (button.getAttribute("aria-busy") === "true") return "wait";
      button.setAttribute("aria-busy", "true");
      button.setAttribute("aria-disabled", "true");
      button.textContent = "Issuing";
      api("POST", "/api/certificates", { kind, name: input.value, showStats: stats.checked })
        .then((res) => {
          issued = res;
          form.closest("dialog").close("ok");
        })
        .catch((err) => {
          button.removeAttribute("aria-busy");
          button.removeAttribute("aria-disabled");
          button.textContent = "Issue the certificate";
          error.textContent = err?.code === "bad_name" ? `Name to print: ${errorMessage(err)}` : errorMessage(err);
          error.hidden = false;
          if (err?.code === "bad_name") {
            input.setAttribute("aria-invalid", "true");
            input.setAttribute("aria-describedby", "claim-name-help claim-name-error");
          }
          input.focus();
        });
      return "wait";
    },
  });
  if (v === "ok" && issued) {
    toastNext("Certificate issued.", "ok", undefined, `[data-kind="${kind}"] [data-card-status]`);
    location.reload();
  }
}
