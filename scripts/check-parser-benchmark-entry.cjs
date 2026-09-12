// Read-only navigation using an existing, locally authorized test session.
const {chromium} = require(process.argv[2] || 'playwright');
const fs = require('node:fs/promises');
const path = require('node:path');
const assert = require('node:assert/strict');
(async () => {
  const auth = JSON.parse(await fs.readFile(process.argv[3], 'utf8'));
  const base = 'http://127.0.0.1:5174';
  const output = path.resolve('artifacts/parser-benchmark/entry-check');
  await fs.mkdir(output, {recursive:true});
  const browser = await chromium.launch({channel:'msedge',headless:true});
  const started = Date.now();
  let guideDismissed = 0;
  let stage = 'context';
  let page;
  let context;
  const mark = value => { stage = value; console.log('Entry check: ' + value); };
  try {
    context = await browser.newContext({viewport:{width:1440,height:1080},reducedMotion:'reduce',storageState:{cookies:[],origins:[{origin:base,localStorage:[
      {name:'weknora_token',value:auth.token},
      {name:'weknora_user',value:JSON.stringify(auth.me.data.user)},
      {name:'weknora_tenant',value:JSON.stringify(auth.me.data.tenant)},
      {name:'weknora_memberships',value:JSON.stringify(auth.me.data.memberships)},
    ]}]}});
    page = await context.newPage();
    page.setDefaultTimeout(15000);
    page.setDefaultNavigationTimeout(30000);
    // A fresh test context may show onboarding after the initial route render.
    // Handle the visible UI instead of altering app state or forcing the click.
    const skip = page.locator('.guide__skip');
    await page.addLocatorHandler(skip, async () => {
      await skip.click();
      guideDismissed += 1;
    });
    mark('workbench_navigation');
    await page.goto(base+'/platform/evaluations',{waitUntil:'domcontentloaded'});
    const entry = page.locator('a[href="http://127.0.0.1:18090/"]');
    mark('entry_visible');
    await entry.waitFor({state:'visible'});
    assert.equal(new URL(page.url()).pathname,'/platform/evaluations');
    assert.equal(await entry.count(),1);
    assert.equal(await entry.getAttribute('href'),'http://127.0.0.1:18090/');
    assert.equal(await entry.locator('svg').count(),1);
    assert.match(await entry.locator('svg').getAttribute('class'),/\blucide\b/);
    // Actionability invokes the guide handler before the evidence screenshot.
    await entry.click({trial:true});
    await page.screenshot({path:path.join(output,'workbench.png'),fullPage:true,mask:[page.locator('.user-button')],maskColor:'#e5e7eb'});
    mark('entry_click');
    const [report] = await Promise.all([context.waitForEvent('page',{timeout:15000}),entry.click()]);
    report.setDefaultTimeout(15000);
    mark('report_load');
    await report.waitForLoadState('domcontentloaded',{timeout:30000});
    await report.locator('#cards').waitFor({state:'visible'});
    await report.waitForFunction(() => document.querySelectorAll('.card').length === 8 && document.querySelectorAll('.sample').length === 100, null, {timeout:15000});
    assert.equal(new URL(report.url()).origin,'http://127.0.0.1:18090');
    assert.equal(await report.locator('.card').count(),8);
    assert.equal(await report.locator('.sample').count(),100);
    await report.screenshot({path:path.join(output,'report.png'),fullPage:true});
    await fs.writeFile(path.join(output,'checks.json'),JSON.stringify({status:'passed',checked_at:new Date().toISOString(),duration_ms:Date.now()-started,authenticated_entry:true,lucide_icon:true,report_navigation:true,report_origin:'http://127.0.0.1:18090',guide_dismissals:guideDismissed,engines:8,pages:100},null,2));
    console.log('Authenticated parser benchmark entry passed');
  } catch(error) {
    const details = {status:'failed',stage,error_class:error.name,checked_at:new Date().toISOString(),
      page_path:page ? new URL(page.url()).pathname : null,
      pages:context ? context.pages().length : 0,
      guide_visible:page ? await page.locator('.guide__skip').isVisible().catch(()=>false) : false};
    await fs.writeFile(path.join(output,'diagnostics.json'),JSON.stringify(details,null,2));
    console.log(JSON.stringify(details));
    throw error;
  } finally { await browser.close(); }
})().catch(error=>{console.error(error.name + ': entry verification failed');process.exitCode=1;});
