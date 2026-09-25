const fs = await import("node:fs");
const path = await import("node:path");
const { base, fixtures, out, documents } = JSON.parse(fs.readFileSync(envFile, "utf8"));
const results = [];
let step = 0;
let retried = false;
const task = await taskSpace("case review e2e flow");
const page = task.page("p1");

async function retryCdp(fn) {
  try { return await fn(); }
  catch (e) {
    if (!/CdpRequestTimeoutError|timed out|Cannot find context with specified id|Execution context was destroyed/i.test(String(e?.message ?? e))) throw e;
    retried = true;
    await page.waitForTimeout(500);
    try { return await fn(); } catch { throw e; }
  }
}
const evaluate = (...args) => retryCdp(() => page.evaluate(...args));
const waitForFunction = (...args) => retryCdp(() => page.waitForFunction(...args));
async function waitForPdfPreview() {
  await waitForFunction(() => {
    const frame = document.querySelector('iframe.doc-frame');
    return frame?.contentDocument?.URL === frame?.src && frame.contentDocument.readyState === 'complete';
  }, undefined, { timeout: 10000 });
}
async function shot(name) {
  if (await evaluate(() => !!document.querySelector('iframe.doc-frame'))) await waitForPdfPreview();
  step += 1;
  const file = path.join(out, `step-${String(step).padStart(2, "0")}-${name}.png`);
  await retryCdp(() => page.screenshot({ path: file }));
  return file;
}
async function check(name, fn) {
  retried = false;
  const before = step;
  let error, detail;
  try { detail = await fn(); }
  catch (e) { error = e; }
  if (step === before) {
    try { await shot(name.toLowerCase().replace(/[^a-z0-9]+/g, '-')); }
    catch (e) { error ??= e; }
  }
  results.push({ step, check: name, ok: !error, retried, detail: error ? String(error?.message ?? error) : detail ?? "" });
}
async function actionRowWidth(width, name) {
  await page.cdp("Emulation.setDeviceMetricsOverride", { width, height: 844, deviceScaleFactor: 1, mobile: false });
  try {
    const layout = await evaluate(() => {
      const row = document.querySelector(".actions");
      row?.scrollIntoView();
      return { note: row?.querySelector(".note-wrap")?.getBoundingClientRect().top,
        buttons: [...(row?.querySelectorAll(".action-buttons button") ?? [])].map((b) => b.getBoundingClientRect().top),
        scrollWidth: document.documentElement.scrollWidth };
    });
    await shot(name);
    if (layout.scrollWidth > width || layout.buttons.some((top) => Math.abs(top - layout.buttons[0]) > 10) || layout.buttons[0] <= layout.note) throw new Error(`action row wraps or overflows at ${width}px: ${JSON.stringify(layout)}`);
  } finally {
    await page.cdp("Emulation.clearDeviceMetricsOverride", {});
  }
}
async function phoneWidth(name) {
  await page.cdp("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 1, mobile: false });
  try {
    const width = await evaluate(() => document.documentElement.scrollWidth);
    await shot(name);
    if (width > 390) throw new Error(`390px viewport has ${width}px scroll width`);
  } finally {
    await page.cdp("Emulation.clearDeviceMetricsOverride", {});
  }
}
const disabled = (label) => evaluate((l) => {
  const b = [...document.querySelectorAll("button")].find((x) => x.textContent.trim() === l);
  if (!b) throw new Error(`button ${l} not found`);
  return b.disabled;
}, label);

try {
  await page.goto(base);
  await shot("warmup");
  await check("390px login has no horizontal overflow", () => phoneWidth("phone-login"));

  await check("wrong password shows the server message", async () => {
    await page.fill("#password", "wrong");
    await page.click("loc=role:button[name='Sign in']");
    await page.waitForSelector("text=Email or password is incorrect. Check both and try again.", { timeout: 5000 });
    await shot("login-error");
  });

  await check("reviewer signs in", async () => {
    await page.fill("#password", "casereview-demo");
    await page.click("loc=role:button[name='Sign in']");
    await page.waitForSelector("text=Select a case from the queue", { timeout: 5000 });
    await shot("signed-in");
  });
  await check("390px empty queue has no horizontal overflow", () => phoneWidth("phone-empty"));

  await check("create case", async () => {
    await page.click("loc=role:button[name='New case']");
    await page.fill("#borrower", "Jordan Alvarez");
    await page.fill("#loan-number", `HB-${Date.now()}`);
    await page.fill("#amount", "410000");
    await page.click("loc=role:button[name='Create case']");
    await page.waitForSelector("loc=role:heading[name='Jordan Alvarez']", { timeout: 5000 });
    await shot("case-created");
  });

  await check("five documents process and recommendation shows", async () => {
    const files = ["w2-2025.pdf", "form-1040.pdf", "form-1003.pdf", "pay-stub.pdf", "bank-statement-lowq.png"].map((f) => path.join(fixtures, f));
    await page.setInputFiles("[data-testid=file-input]", files);
    await page.waitForSelector("text=5 of 5 processed", { timeout: 20000 });
    await page.waitForSelector("text=Recommendation: needs review", { timeout: 10000 });
    await shot("processed");
  });

  await check("one review case uses singular topbar grammar", async () => {
    const state = await evaluate(() => ({ cases: document.querySelectorAll('.queue-item').length,
      label: document.querySelector('.topbar-meta .pill')?.textContent.trim() }));
    if (state.cases !== 1 || state.label !== '1 needs review') throw new Error(JSON.stringify(state));
  });

  await check("390px five-document case scrolls its strip, not the page", async () => {
    await page.cdp("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 1, mobile: false });
    try {
      const state = await evaluate(() => ({ width: document.documentElement.scrollWidth,
        docs: document.querySelectorAll('.doc-strip button[aria-pressed]').length,
        stripWidth: document.querySelector('.doc-strip')?.clientWidth,
        stripContent: document.querySelector('.doc-strip')?.scrollWidth,
        queue: getComputedStyle(document.querySelector('.queue')).display,
        allCases: getComputedStyle(document.querySelector('.all-cases')).display }));
      await evaluate(() => window.scrollTo(0, 0));
      await shot("phone-five-documents");
      if (state.width > 390 || state.docs !== 5 || state.stripContent <= state.stripWidth || state.queue !== 'none' || state.allCases === 'none') throw new Error(JSON.stringify(state));
    } finally { await page.cdp("Emulation.clearDeviceMetricsOverride", {}); }
  });

  await check("case blocker is in the first viewport", async () => {
    await page.waitForSelector(".case-status:has-text('1 field to verify')", { timeout: 5000 });
    const pos = await evaluate(() => ({ top: document.querySelector('.case-status')?.getBoundingClientRect().top, height: innerHeight }));
    if (pos.top >= pos.height) throw new Error(`blocker below fold: ${JSON.stringify(pos)}`);
    await shot("blocker-first-viewport");
  });
  await check("1024px document and fields stay side by side", async () => {
    await page.cdp("Emulation.setDeviceMetricsOverride", { width: 1024, height: 768, deviceScaleFactor: 1, mobile: false });
    try {
      const tops = await evaluate(() => [...document.querySelectorAll('.columns > *')].map((el) => el.getBoundingClientRect().top));
      await shot("side-by-side-1024");
      if (Math.abs(tops[0] - tops[1]) > 10) throw new Error(`columns stacked: ${tops}`);
    } finally { await page.cdp("Emulation.clearDeviceMetricsOverride", {}); }
  });
  await check("decision actions stay on one row at 900px", () => actionRowWidth(900, "actions-900"));
  await check("decision actions stay on one row at 1360px", () => actionRowWidth(1360, "actions-1360"));

  await check("approve blocked while a field is flagged", async () => {
    if (!(await disabled("Approve"))) throw new Error("Approve was enabled with an unresolved flag");
  });

  await check("reviewer corrects the flagged field", async () => {
    const sel = '[aria-label="Ending balance, flagged for review"]';
    await page.fill(sel, "18432");
    await page.press(sel, "Enter");
    await waitForFunction(() => [...document.querySelectorAll("button")].find((b) => b.textContent.trim() === "Approve")?.disabled === false, undefined, { timeout: 5000 });
    await page.click("loc=role:button[name='Bank statement']");
    await page.waitForSelector("text=Verified by reviewer", { timeout: 5000 });
    if (await disabled("Approve")) throw new Error("Approve still disabled after correction");
    await shot("field-corrected");
  });

  await check("approve records the decision and audit trail", async () => {
    await page.fill("#decision-note", "Verified ending balance against source scan");
    await page.click("loc=role:button[name='Approve']");
    await waitForFunction(() => document.querySelector(".actions strong")?.textContent === "Approved", undefined, { timeout: 5000 });
    await waitForFunction(() => /field edited/i.test(document.querySelector(".audit")?.textContent ?? ""), undefined, { timeout: 5000 });
    const inputs = await evaluate(() => document.querySelectorAll('[aria-label$="flagged for review"]').length);
    if (inputs !== 0) throw new Error("fields still editable after decision");
    await shot("approved");
  });
  await check("create a retry case", async () => {
    await page.click("loc=role:button[name='New case']");
    await page.fill("#borrower", "Retry Example");
    await page.fill("#loan-number", `HB-RETRY-${Date.now()}`);
    await page.fill("#amount", "250000");
    await page.click("loc=role:button[name='Create case']");
    await page.waitForSelector("loc=role:heading[name='Retry Example']", { timeout: 5000 });
    await shot("retry-case");
  });

  await check("failed document explains retry", async () => {
    await page.setInputFiles("[data-testid=file-input]", path.join(fixtures, "pay-stub-fail-once.pdf"));
    await page.waitForSelector("text=This document couldn't be read, so its type is unknown. Retry to process it again.", { timeout: 10000 });
    await page.waitForSelector("text=simulated parser outage", { timeout: 5000 });
    await page.waitForSelector("loc=role:button[name='Retry']", { timeout: 5000 });
    const failure = await evaluate(() => ({ thumb: [...document.querySelectorAll('.doc-thumb')].some((b) => b.textContent.trim() === 'pay-stub-fail-once (failed)'),
      empty: !!document.querySelector('[aria-label="Pay stub not uploaded"]'),
      friendly: document.body.innerText.includes("This document couldn't be read, so its type is unknown. Retry to process it again.") }));
    if (!failure.thumb || !failure.empty || !failure.friendly) throw new Error(JSON.stringify(failure));
    await shot("document-failed");
  });

  await check("retry processes failed document", async () => {
    await page.click("loc=role:button[name='Retry']");
    await page.waitForSelector("text=1 of 5 processed", { timeout: 10000 });
    const recovered = await evaluate(() => ({ failure: document.body.innerText.includes("This document couldn't be read"),
      thumb: [...document.querySelectorAll('.doc-thumb')].some((b) => b.textContent.trim() === 'Pay stub'),
      empty: !!document.querySelector('[aria-label="Pay stub not uploaded"]') }));
    if (recovered.failure || !recovered.thumb || recovered.empty) throw new Error(JSON.stringify(recovered));
    await shot("document-retried");
  });

  await check("second low-quality file presents its original value", async () => {
    await page.click("loc=role:button[name='New case']");
    await page.fill("#borrower", "Confirm Example");
    await page.fill("#loan-number", `HB-CONFIRM-${Date.now()}`);
    await page.fill("#amount", "410000");
    await page.click("loc=role:button[name='Create case']");
    await page.waitForSelector("loc=role:heading[name='Confirm Example']", { timeout: 5000 });
    const files = ["w2-2025.pdf", "form-1040.pdf", "form-1003.pdf", "pay-stub.pdf", "bank-statement-lowq.png"].map((f) => path.join(fixtures, f));
    await page.setInputFiles("[data-testid=file-input]", files);
    await page.waitForSelector("text=5 of 5 processed", { timeout: 20000 });
    await page.waitForSelector("loc=role:button[name='Confirm value']", { timeout: 5000 });
    const value = await evaluate(() => document.querySelector('[aria-label="Ending balance, flagged for review"]')?.value);
    if (value !== "$18,482.00") throw new Error(`original value not formatted: ${value}`);
    if (!(await disabled("Approve"))) throw new Error("Approve enabled before confirmation");
    await shot("unchanged-flagged");
  });

  await check("confirming unchanged value enables approval", async () => {
    await page.click("loc=role:button[name='Confirm value']");
    await waitForFunction(() => [...document.querySelectorAll("button")].find((b) => b.textContent.trim() === "Approve")?.disabled === false, undefined, { timeout: 5000 });
    await page.waitForSelector("text=Verified by reviewer", { timeout: 5000 });
    const value = await evaluate(() => [...document.querySelectorAll(".field-row")].find((r) => r.textContent.includes("Ending balance"))?.querySelector(".field-value")?.textContent);
    if (value !== "$18,482.00") throw new Error(`confirmed value changed: ${value}`);
    const state = await evaluate(() => ({ focus: document.activeElement?.textContent?.trim(),
      reason: document.querySelector('.action-blocker')?.textContent }));
    if (state.focus !== 'Approve' || !state.reason?.startsWith('Check before approving: DTI is 42.8%, within 2 points of the 43% limit')) throw new Error(JSON.stringify(state));
    await shot("unchanged-confirmed");
  });

  await check("near-limit approval is enabled with a warning", async () => {
    if (await disabled('Approve')) throw new Error('Approve disabled for near-limit case');
    await shot('near-limit-warning');
  });

  await check("confirming the last flag focuses a blocked reason when approval stays disabled", async () => {
    await page.click("loc=role:button[name='New case']");
    await page.fill("#borrower", "Incomplete Review Example");
    await page.fill("#loan-number", `HB-INCOMPLETE-${Date.now()}`);
    await page.fill("#amount", "180000");
    await page.click("loc=role:button[name='Create case']");
    await page.waitForSelector("loc=role:heading[name='Incomplete Review Example']", { timeout: 5000 });
    await page.setInputFiles("[data-testid=file-input]", path.join(fixtures, "bank-statement-lowq.png"));
    await page.waitForSelector("loc=role:button[name='Confirm value']", { timeout: 10000 });
    await page.click("loc=role:button[name='Confirm value']");
    await page.waitForSelector("text=Verified by reviewer", { timeout: 5000 });
    const state = await evaluate(() => ({ disabled: document.querySelector('#approve-button')?.disabled,
      focused: document.activeElement?.classList.contains('action-blocker'),
      reason: document.activeElement?.textContent, tabIndex: document.activeElement?.tabIndex }));
    if (!state.disabled || !state.focused || state.tabIndex !== -1 || !state.reason?.includes('Upload the W-2, Form 1040, Form 1003 and pay stub before approving.')) throw new Error(JSON.stringify(state));
    await shot('blocked-reason-focused');
  });

  await check("synthetic PDFs drive distinct decisions and blockers", async () => {
    for (const [scenario, borrower, status] of [
      ["standard", "Jordan Alvarez", "Ready for decision"],
      ["above_limit", "Taylor Brooks", "DTI above 43%"],
      ["unsupported_doc", "Casey Rivera", "1 unsupported document"],
      ["tampered_statement", "Morgan Ellis", "Check document authenticity on the bank statement"],
    ]) {
      await page.click("loc=role:button[name='New case']");
      await page.fill("#borrower", borrower);
      await page.fill("#loan-number", `HB-${scenario}-${Date.now()}`);
      await page.fill("#amount", "410000");
      await page.click("loc=role:button[name='Create case']");
      await page.waitForSelector(`loc=role:heading[name='${borrower}']`, { timeout: 5000 });
      const dir = path.join(documents, scenario);
      await page.setInputFiles("[data-testid=file-input]", fs.readdirSync(dir).filter((f) => f.endsWith(".pdf")).map((f) => path.join(dir, f)));
      await page.waitForSelector("text=5 of 5 processed", { timeout: 20000 });
      if (scenario !== "tampered_statement") await page.waitForSelector(`.case-status:has-text('${status}')`, { timeout: 10000 });
      if (scenario === "tampered_statement") {
        await check("tampered statement queue and header show specific warn blocker", async () => {
          await page.waitForSelector(`.queue-loan:has-text('${status}')`, { timeout: 10000 });
          await page.waitForSelector(`.case-status:has-text('${status}')`, { timeout: 10000 });
          const warning = await evaluate((name) => [...document.querySelectorAll('.queue-item')].find((el) => el.textContent.includes(name))?.querySelector('.dot.warn')?.getAttribute('aria-label'), borrower);
          if (warning !== status) throw new Error(`tampered queue warning missing: ${warning}`);
        });
      }
      if (scenario === "standard") {
        if (await disabled("Approve")) throw new Error("Approve disabled for standard file");
        const header = await evaluate(() => [...document.querySelector('.case-status').children].map((el) => el.textContent.trim()));
        if (header.length !== 1 || header[0] !== "Ready for decision") throw new Error(`duplicate case status: ${JSON.stringify(header)}`);
        await check("all five standard PDF previews load on selection", async () => {
          for (const name of ["Form 1040", "W‑2", "Form 1003", "Bank statement", "Pay stub"]) {
            const alreadySelected = await evaluate((label) => document.querySelector('.doc-strip button[aria-pressed="true"]')?.textContent.trim() === label, name);
            if (!alreadySelected) {
              await evaluate(() => {
                window.__previewLoaded = false;
                document.querySelector('iframe.doc-frame').addEventListener('load', () => { window.__previewLoaded = true; }, { once: true });
              });
              await page.click(`loc=role:button[name='${name}']`);
              await waitForFunction(() => window.__previewLoaded === true, undefined, { timeout: 10000 });
            }
            await waitForPdfPreview();
            const preview = await evaluate(() => ({ title: document.querySelector('iframe.doc-frame')?.title,
              url: document.querySelector('iframe.doc-frame')?.src }));
            if (preview.title !== `${name} source`) throw new Error(`${name} preview mismatch: ${JSON.stringify(preview)}`);
            const response = await page.fetch(preview.url);
            if (response.status !== 200 || !response.headers['content-type']?.startsWith('application/pdf')) throw new Error(`${name} PDF response: ${response.status} ${response.headers['content-type']}`);
            await shot(`preview-${name.toLowerCase().replace(/[^a-z0-9]+/g, '-')}`);
          }
        });
      }
      if (scenario === "unsupported_doc") {
        const warning = await evaluate(() => [...document.querySelectorAll(".queue-item")].find((el) => el.textContent.includes("Casey Rivera"))?.querySelector(".dot.warn")?.getAttribute("aria-label"));
        if (warning !== status) throw new Error(`unsupported queue warning missing: ${warning}`);
      }
      await shot(`synthetic-${scenario}`);
    }
  });

  await check("two-document case explains missing file and blocks approval", async () => {
    await page.click("loc=role:button[name='New case']");
    await page.fill("#borrower", "Missing Documents Example");
    await page.fill("#loan-number", `HB-MISSING-${Date.now()}`);
    await page.fill("#amount", "180000");
    await page.click("loc=role:button[name='Create case']");
    await page.waitForSelector("loc=role:heading[name='Missing Documents Example']", { timeout: 5000 });
    await page.setInputFiles("[data-testid=file-input]", ["w2-2025.pdf", "form-1040.pdf"].map((f) => path.join(fixtures, f)));
    await page.waitForSelector("text=2 of 5 processed", { timeout: 10000 });
    await page.waitForSelector("text=Upload the Form 1003, pay stub and bank statement before approving.", { timeout: 5000 });
    for (const label of ["Form 1003", "Pay stub", "Bank statement"]) {
      if (!(await evaluate((l) => !!document.querySelector(`[aria-label="${l} not uploaded"]`), label))) throw new Error(`${label} empty slot missing`);
    }
    if (!(await disabled("Approve"))) throw new Error("Approve enabled with three documents missing");
    await shot("missing-documents");
  });
} finally {
  const failed = results.filter((r) => !r.ok).length;
  const lines = results.map((r) => `${r.ok ? "PASS" : "FAIL"} ${r.check}${r.ok ? "" : ` — ${r.detail}`}`);
  lines.push(`Result: ${failed ? "FAIL" : "PASS"}; evidence: ${out}`);
  try {
    fs.mkdirSync(out, { recursive: true });
    fs.writeFileSync(path.join(out, "summary.txt"), lines.join("\n") + "\n");
    fs.writeFileSync(path.join(out, "results.json"), JSON.stringify(results, null, 2));
    console.log(lines.join("\n"));
  } finally {
    await task.finish({ keep: [] });
  }
  if (failed) process.exitCode = 1;
}
