// certificates.js: claim, issue again and share a certificate.
import { api, errorMessage } from "../api.js";
import { h } from "../ui/dom.js";
import { openDialog } from "../ui/dialog.js";
import { toast, toastNext } from "../ui/toast.js";
import { zero } from "../ui/zero.js";

const page = document.querySelector("[data-display-name]");
const displayName = page?.dataset.displayName || "";

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
  for (const button of card.querySelectorAll("[data-claim]")) {
    button.addEventListener("click", () => claim(card, button.dataset.claim === "again"));
  }
  for (const button of card.querySelectorAll("[data-copy-link]")) {
    button.addEventListener("click", async () => {
      // An earlier certificate's block carries its own link.
      const path = button.closest("[data-earlier-url]")?.dataset.earlierUrl || card.dataset.url;
      const url = new URL(path, location.origin).href;
      try {
        await navigator.clipboard.writeText(url);
        toast("Verify link copied.");
      } catch {
        toast(`The browser did not allow the copy. The link is ${url}`, { kind: "info" });
      }
    });
  }
}

/** A title-block cell: a label over a value (VESPER.md 6.7). */
const cell = (label, value) => h("div", {}, h("dt", { class: "gpw-lbl" }, label), value);

async function claim(card, again) {
  const { kind, label, title, range, summary } = card.dataset;
  const input = h("input", { class: "text-input", id: "claim-name", name: "name", maxlength: 70, required: true, value: card.dataset.name || displayName, autocomplete: "name", "aria-describedby": "claim-name-help" });
  const error = h("p", { class: "field-error", id: "claim-name-error", hidden: true });
  const preview = h("dd", {});
  const previewNote = h("p", { class: "field-error", hidden: true });
  // Only a refused name belongs under the field; a limit or a server error
  // is about the claim, so it shows above the buttons.
  const status = h("p", { class: "form-status", role: "status" });
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
    summary ? h("p", {}, summary) : null,
    h("div", { class: "field" },
      h("label", { for: "claim-name" }, "Name to print"),
      input,
      h("p", { class: "help", id: "claim-name-help" }, "Use 1 to 70 characters. The PDF and the verify page show this name."),
      error),
    h("label", { class: "gpw-cb", for: "claim-stats" }, stats,
      h("span", {},
        h("span", { class: "gpw-cb-t" }, "Show my activity on the certificate and the verify page"),
        h("span", { class: "help", id: "claim-stats-help" }, "Active time, runs, and the hints, explanations, walkthroughs and solutions you revealed."))),
    h("div", {},
      h("p", { class: "gpw-lbl gpw-dlg-cap", id: "claim-preview" }, "As it will print"),
      h("dl", { class: "gpw-tb gpw-tb-3", "aria-labelledby": "claim-preview" },
        cell("Name", preview),
        cell("Certificate", h("dd", {}, zero(title || ""))),
        cell("Exercises", h("dd", {}, zero(range || "")))),
      previewNote),
    status,
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
      status.textContent = "";
      delete status.dataset.kind;
      api("POST", "/api/certificates", { kind, name: input.value, showStats: stats.checked })
        .then((res) => {
          issued = res;
          form.closest("dialog").close("ok");
        })
        .catch((err) => {
          button.removeAttribute("aria-busy");
          button.removeAttribute("aria-disabled");
          button.textContent = "Issue the certificate";
          if (err?.code !== "bad_name") {
            status.dataset.kind = "error";
            status.textContent = errorMessage(err);
            button.focus();
            return;
          }
          error.textContent = `Name to print: ${errorMessage(err)}`;
          error.hidden = false;
          input.setAttribute("aria-invalid", "true");
          input.setAttribute("aria-describedby", "claim-name-help claim-name-error");
          input.focus();
        });
      return "wait";
    },
  });
  if (v === "ok" && issued) {
    // A new tag fades in on the ring at the next view of the course page
    // (dashboard.js); a certificate issued again is not a new tag.
    if (!again && kind !== "course") {
      try {
        sessionStorage.setItem("gpw-ring-new", "1");
      } catch {
        // Storage blocked: the tag shows without the fade.
      }
    }
    toastNext("Certificate issued.", "ok", issued.pdf ? { label: "Open the PDF", href: issued.pdf } : undefined);
    location.reload();
  }
}
