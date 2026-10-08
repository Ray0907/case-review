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
    const viewer = document.querySelector('.pdf-viewer');
    const canvases = [...document.querySelectorAll('.pdf-viewer canvas')];
    return viewer?.dataset.state === 'ready' && canvases.length > 0 && canvases.every((c) => c.width > 0 && c.height > 0);
  }, undefined, { timeout: 10000 });
}
async function shot(name) {
  if (await evaluate(() => !!document.querySelector('.pdf-viewer canvas'))) await waitForPdfPreview();
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

  await check("cross-check charts match their table view", async () => {
    await page.waitForSelector('.analysis [data-chart]', { timeout: 5000 });
    const before = await evaluate(() => [...document.querySelectorAll('.analysis [data-chart]')].map((c) => ({
      id: c.dataset.chart, bars: c.querySelectorAll('.bar-row').length,
      values: [...c.querySelectorAll('.bar-value')].map((v) => v.textContent.trim()) })));
    if (before.length === 0 || before.some((c) => c.bars < 2)) throw new Error(`charts: ${JSON.stringify(before)}`);
    await evaluate(() => document.querySelector('.analysis .chart-toggle').click());
    const table = await evaluate(() => [...document.querySelector('.analysis [data-chart] .chart-table tbody').querySelectorAll('td')].map((td) => td.textContent.trim()));
    if (JSON.stringify(table) !== JSON.stringify(before[0].values)) throw new Error(`table ${JSON.stringify(table)} != chart ${JSON.stringify(before[0].values)}`);
    await evaluate(() => document.querySelector('.analysis .chart-toggle').click());
    await shot("cross-check-charts");
  });

  await check("field click grounds the image and clears without stale overlays", async () => {
    await page.click("loc=role:button[name='Show source for Ending balance']");
    await page.waitForSelector('[data-testid="source-highlight"]', { timeout: 10000 });
    const geometry = await evaluate(() => {
      const box = document.querySelector('[data-testid="source-highlight"]').getBoundingClientRect();
      const page = document.querySelector('.source-page').getBoundingClientRect();
      return { left: box.left >= page.left, top: box.top >= page.top, right: box.right <= page.right + 1, bottom: box.bottom <= page.bottom + 1 };
    });
    if (Object.values(geometry).some((v) => !v)) throw new Error(JSON.stringify(geometry));
    await shot('grounded-image');
    await page.click("loc=role:button[name='Clear highlight']");
    if (await evaluate(() => !!document.querySelector('[data-testid="source-highlight"]'))) throw new Error('stale image overlay');
  });

  await check("field click renders and highlights the PDF page", async () => {
    await page.click("loc=role:button[name='W‑2']");
    await page.click("loc=role:button[name='Show source for Box 1 wages']");
    await page.waitForSelector('[data-testid="source-highlight"]', { timeout: 10000 });
    const canvas = await evaluate(() => {
      const c = document.querySelector('.source-page canvas');
      return c && c.width > 0 && c.height > 0;
    });
    if (!canvas) throw new Error('PDF page was not rendered');
    await shot('grounded-pdf');
    await phoneWidth('grounded-pdf-phone');
    await page.click("loc=role:button[name='Bank statement']");
    if (await evaluate(() => !!document.querySelector('[data-testid="source-highlight"]'))) throw new Error('stale document overlay');
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
    const early = await evaluate(async () => {
      const id = location.pathname.split('/').pop();
      const res = await fetch(`/api/cases/${id}/ask`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ question: 'What was the DTI?' }) });
      return { status: res.status, launcher: !!document.querySelector('.ask-launch') };
    });
    if (early.status !== 409 || early.launcher) throw new Error(`open case must not accept questions: ${JSON.stringify(early)}`);
    await page.fill("#decision-note", "Verified ending balance against source scan");
    await evaluate(() => {
      window.__originalFetch = window.fetch;
      window.fetch = async (...args) => {
        if (String(args[0]).endsWith('/decision')) await new Promise((resolve) => { window.__releaseDecision = resolve; });
        return window.__originalFetch(...args);
      };
    });
    try {
      await page.click("loc=role:button[name='Approve']");
      await page.waitForSelector('text=Recording decision…', { timeout: 2000 });
      const pending = await evaluate(() => document.querySelector('#decision-note').disabled &&
        [...document.querySelectorAll('.action-buttons button')].every((b) => b.disabled));
      if (!pending) throw new Error('Decision controls remain enabled during submission');
    } finally {
      await evaluate(() => { window.fetch = window.__originalFetch; window.__releaseDecision?.(); });
    }
    await waitForFunction(() => document.querySelector(".actions strong")?.textContent === "Approved", undefined, { timeout: 5000 });
    await waitForFunction(() => /field edited/i.test(document.querySelector(".audit")?.textContent ?? ""), undefined, { timeout: 5000 });
    const inputs = await evaluate(() => document.querySelectorAll('[aria-label$="flagged for review"]').length);
    if (inputs !== 0) throw new Error("fields still editable after decision");
    await shot("approved");
  });

  await check("approval shows a decision receipt in view with a way to continue", async () => {
    const receipt = await evaluate(() => {
      const el = document.querySelector('[aria-label="Decision recorded"]');
      const box = el?.getBoundingClientRect();
      return { found: !!el, top: box?.top, bottom: box?.bottom, vh: innerHeight, text: el?.textContent ?? '', focused: document.activeElement === el,
        actions: [...(el?.querySelectorAll('button') ?? [])].map((b) => b.textContent.trim()) };
    });
    if (!receipt.found || receipt.bottom < 0 || receipt.top > receipt.vh) throw new Error(`receipt not in view: ${JSON.stringify(receipt)}`);
    if (!/Approved/.test(receipt.text) || !/Recorded by/.test(receipt.text)) throw new Error(`receipt content: ${receipt.text}`);
    if (!receipt.focused) throw new Error('focus did not move to the receipt');
    if (!receipt.actions.includes('Back to queue')) throw new Error(`receipt actions: ${receipt.actions}`);
    await shot("decision-receipt");
  });

  await check("closed case answers a question with sources the reviewer can open", async () => {
    await page.click("loc=role:button[name='Ask about this case']");
    await page.waitForSelector('#ask-window', { timeout: 3000 });
    if (!(await evaluate(() => document.activeElement?.id === 'ask-input'))) throw new Error('focus did not move to the question box');
    await evaluate(() => {
      window.__lengths = new Set();
      new MutationObserver(() => {
        const bubbles = [...document.querySelectorAll('.ask-bubble-ai')];
        const last = bubbles.at(-1);
        if (last) window.__lengths.add(last.textContent.length);
      }).observe(document.querySelector('.ask-log'), { childList: true, subtree: true, characterData: true });
    });
    await page.click("loc=role:button[name='What was the debt-to-income ratio?']");
    await waitForFunction(() => /debt-to-income ratio was.*43%/i.test(document.querySelector('.ask-bubble-ai')?.textContent ?? ''), undefined, { timeout: 8000 });
    const steps = await evaluate(() => window.__lengths.size);
    if (steps < 3) throw new Error(`answer appeared in ${steps} step(s); it should stream`);
    const sources = await evaluate(() => [...document.querySelectorAll('.ask-source')].map((b) => b.textContent.trim()));
    if (sources.length === 0) throw new Error('answer came without sources');
    await shot("ask-window");
    await page.click("loc=role:button[name='Close case assistant']");
    if (await evaluate(() => !!document.querySelector('#ask-window'))) throw new Error('window stayed open');
    if (!(await evaluate(() => document.activeElement?.classList.contains('ask-launch')))) throw new Error('focus did not return to the launcher');
  });

  await check("the assistant lives inside its case and keeps one conversation per case", async () => {
    const caseId = await evaluate(() => location.pathname.split('/').pop());
    // Earlier runs leave their own keys in this browser profile; only this case's conversation matters here.
    const stored = await evaluate((id) => Object.keys(localStorage).filter((k) => k.startsWith('case-review:ask:') && k.endsWith(`:${id}`)), caseId);
    if (stored.length !== 1) throw new Error(`conversations stored for this case: ${JSON.stringify(stored)}`);
    await page.click("loc=role:button[name='New case']");
    await page.waitForSelector("text=Create case", { timeout: 3000 });
    if (await evaluate(() => !!document.querySelector('.ask-launch'))) throw new Error('assistant visible on the new case form');
    await page.click("loc=role:button[name='Cancel']");
    await page.click(`.queue-item:has-text("Jordan")`);
    await page.waitForSelector('.ask-launch', { timeout: 5000 });
    await page.click("loc=role:button[name='Ask about this case']");
    const restored = await evaluate(() => document.querySelectorAll('.ask-message-user').length);
    if (restored !== 1) throw new Error(`conversation not restored: ${restored} user message(s)`);
    await page.click("loc=role:button[name='Close case assistant']");
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
              await page.click(`loc=role:button[name='${name}']`);
            }
            await waitForPdfPreview();
            const preview = await evaluate(() => ({ title: document.querySelector('.pdf-viewer')?.getAttribute('aria-label'),
              url: document.querySelector('.pdf-viewer')?.dataset.url }));
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
  await check("phone decision actions are fully visible without horizontal scrolling", async () => {
    for (const width of [320, 390]) {
      await page.cdp("Emulation.setDeviceMetricsOverride", { width, height: 844, deviceScaleFactor: 1, mobile: false });
      try {
        const layout = await evaluate(() => {
          const row = document.querySelector('.action-buttons');
          row.scrollIntoView({ block: 'center' });
          const bounds = row.getBoundingClientRect();
          return { width: row.clientWidth, content: row.scrollWidth,
            visible: [...row.querySelectorAll('button')].every((b) => b.getBoundingClientRect().right <= bounds.right + 1) };
        });
        await shot(`phone-actions-${width}`);
        if (layout.content > layout.width + 1 || !layout.visible) throw new Error(JSON.stringify(layout));
        const fontSize = await evaluate(() => parseFloat(getComputedStyle(document.querySelector('.note')).fontSize));
        if (fontSize < 16) throw new Error(`Mobile input font is ${fontSize}px`);
      } finally { await page.cdp('Emulation.clearDeviceMetricsOverride', {}); }
    }
  });

  await check("switching documents discards the previous document's unsaved edit", async () => {
    await page.click("loc=css:.doc-strip button:has-text('Form 1040')");
    await page.click("loc=role:button[name='Edit Tax year']");
    await page.fill('#field-tax_year', '1999');
    await page.click("loc=css:.doc-strip button:has-text('W‑2')");
    const stale = await evaluate(() => document.querySelector('#field-tax_year')?.value);
    if (stale != null) throw new Error(`Unsaved tax year leaked to W-2: ${stale}`);
  });

  await check("field errors are linked to the input and editing has a visible cancel", async () => {
    await page.click("loc=role:button[name='Edit Box 1 wages']");
    await page.fill('#field-box1_wages', 'invalid');
    await page.click("loc=role:button[name='Confirm value']");
    const state = await evaluate(() => {
      const input = document.querySelector('#field-box1_wages');
      return { invalid: input.getAttribute('aria-invalid'),
        error: document.getElementById(input.getAttribute('aria-describedby'))?.textContent };
    });
    if (state.invalid !== 'true' || !state.error?.includes('Enter a valid number')) throw new Error(JSON.stringify(state));
    await page.click("loc=role:button[name='Cancel edit']");
    if (await evaluate(() => !!document.querySelector('#field-box1_wages'))) throw new Error('Cancel left the editor open');
    await waitForFunction(() => document.activeElement?.getAttribute('aria-label') === 'Edit Box 1 wages', undefined, { timeout: 3000 });
  });

  await check("case loading and failure show feedback with a working retry", async () => {
    const casePath = new URL(await page.url()).pathname.replace('/cases/', '/api/cases/');
    await page.click('.queue-item:has-text("Confirm Example")');
    await page.waitForSelector("loc=role:heading[name='Confirm Example']", { timeout: 5000 });
    await evaluate((casePath) => {
      window.__originalFetch = window.fetch;
      window.fetch = async (...args) => {
        if (String(args[0]) === casePath) {
          await new Promise((resolve) => { window.__releaseCase = resolve; });
          return new Response(JSON.stringify({ error: 'Temporary case failure' }), { status: 503 });
        }
        return window.__originalFetch(...args);
      };
    }, casePath);
    try {
      await page.click('.queue-item:has-text("Missing Documents Example")');
      await page.waitForSelector('text=Loading case…', { timeout: 2000 });
      await evaluate(() => window.__releaseCase());
      await page.waitForSelector('text=Could not load this case.', { timeout: 3000 });
    } finally {
      await evaluate(() => { window.fetch = window.__originalFetch; window.__releaseCase?.(); });
    }
    await page.click("loc=role:button[name='Retry case']");
    await page.waitForSelector("loc=role:heading[name='Missing Documents Example']", { timeout: 5000 });
  });

  await check("an older case response cannot replace the currently selected case", async () => {
    const delayedPath = new URL(await page.url()).pathname.replace('/cases/', '/api/cases/');
    await page.click('.queue-item:has-text("Confirm Example")');
    await page.waitForSelector("loc=role:heading[name='Confirm Example']", { timeout: 5000 });
    await evaluate((delayedPath) => {
      window.__originalFetch = window.fetch;
      window.__delayedCaseComplete = false;
      delete window.__releaseCase;
      window.fetch = async (...args) => {
        const response = await window.__originalFetch(...args);
        if (String(args[0]) === delayedPath) {
          await new Promise((resolve) => { window.__releaseCase = resolve; });
          window.__delayedCaseComplete = true;
        }
        return response;
      };
    }, delayedPath);
    try {
      await page.click('.queue-item:has-text("Missing Documents Example")');
      await waitForFunction(() => !!window.__releaseCase, undefined, { timeout: 3000 });
      await page.click('.queue-item:has-text("Confirm Example")');
      await page.waitForSelector("loc=role:heading[name='Confirm Example']", { timeout: 5000 });
      await evaluate(() => window.__releaseCase());
      await waitForFunction(() => window.__delayedCaseComplete, undefined, { timeout: 3000 });
      // Yield to the response's JSON parsing and React's render before checking.
      await evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
      const heading = await evaluate(() => document.querySelector('.case-head h1')?.textContent);
      if (heading !== 'Confirm Example') throw new Error(`Stale case replaced current case: ${heading}`);
    } finally {
      await evaluate(() => { window.fetch = window.__originalFetch; window.__releaseCase?.(); delete window.__releaseCase; });
    }
  });

  await check("saving a field after changing cases cannot overwrite the new case", async () => {
    await page.click('.queue-item:has-text("Confirm Example")');
    await page.waitForSelector("loc=role:heading[name='Confirm Example']", { timeout: 5000 });
    await page.click("loc=css:.doc-strip button:has-text('Bank statement')");
    await page.click("loc=role:button[name='Edit Ending balance']");
    await page.fill('#field-ending_balance', '18484');
    await evaluate(() => {
      window.__originalFetch = window.fetch;
      window.__saveComplete = false;
      window.fetch = async (...args) => {
        if (args[1]?.method === 'PATCH') {
          await new Promise((resolve) => { window.__releaseSave = resolve; });
          const response = await window.__originalFetch(...args);
          window.__saveComplete = true;
          return response;
        }
        return window.__originalFetch(...args);
      };
    });
    try {
      await page.click("loc=role:button[name='Confirm value']");
      await waitForFunction(() => !!window.__releaseSave, undefined, { timeout: 3000 });
      await page.click('.queue-item:has-text("Missing Documents Example")');
      await page.waitForSelector("loc=role:heading[name='Missing Documents Example']", { timeout: 5000 });
      await evaluate(() => window.__releaseSave());
      await waitForFunction(() => window.__saveComplete, undefined, { timeout: 3000 });
      await evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));
      const heading = await evaluate(() => document.querySelector('.case-head h1')?.textContent);
      if (heading !== 'Missing Documents Example') throw new Error(`Field save replaced the selected case: ${heading}`);
    } finally {
      await evaluate(() => { window.fetch = window.__originalFetch; window.__releaseSave?.(); });
    }
  });

  await check("a queue refresh failure preserves cases and offers a working retry", async () => {
    await evaluate(() => {
      window.__originalFetch = window.fetch;
      window.fetch = (...args) => String(args[0]) === '/api/cases' && args[1]?.method !== 'POST'
        ? Promise.resolve(new Response(JSON.stringify({ error: 'Temporary queue failure' }), { status: 503 }))
        : window.__originalFetch(...args);
    });
    try {
      await page.click("loc=role:button[name='New case']");
      await page.fill('#borrower', 'Queue Recovery Example');
      await page.fill('#loan-number', `HB-QUEUE-${Date.now()}`);
      await page.fill('#amount', '180000');
      await page.click("loc=role:button[name='Create case']");
      await page.waitForSelector('text=Could not load the review queue.', { timeout: 5000 });
      const count = await evaluate(() => document.querySelectorAll('.queue-item').length);
      if (!count) throw new Error('Queue failure removed previously loaded cases');
    } finally { await evaluate(() => { window.fetch = window.__originalFetch; }); }
    await page.click("loc=role:button[name='Retry queue']");
    await page.waitForSelector('.queue-item:has-text("Queue Recovery Example")', { timeout: 5000 });
    if (await evaluate(() => !!document.querySelector('.load-error'))) throw new Error('Retry did not clear the queue error');
  });

  await check("upload errors are announced and do not carry into another case", async () => {
    const invalidFile = path.join(out, 'invalid.txt');
    fs.writeFileSync(invalidFile, 'Not a supported document');
    await page.setInputFiles('[data-testid=file-input]', invalidFile);
    await page.waitForSelector('.form-error[role="alert"]:has-text("invalid.txt")', { timeout: 5000 });
    await page.click('.queue-item:has-text("Confirm Example")');
    await page.waitForSelector("loc=role:heading[name='Confirm Example']", { timeout: 5000 });
    if (await evaluate(() => [...document.querySelectorAll('.form-error')].some((el) => el.textContent.includes('invalid.txt')))) throw new Error('Upload error carried into another case');
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
