// Summarize acceptance stage results for the GitHub Actions workflow.
// Called by actions/github-script — receives { github, context, core }.

module.exports = async ({ github, context, core }) => {
  const jobs = await listJobs(github, context);
  const results = classifyJobs(jobs);
  const summary = formatSummary(results);

  core.summary.addRaw(summary);
  await core.summary.write();

  if (results.failed.length > 0) {
    core.setFailed(`${results.failed.length} job(s) failed`);
  } else if (results.rateLimited.length > 0) {
    core.setFailed(`${results.rateLimited.length} job(s) hit rate limit — retry needed`);
  }
};

const SKIP_JOBS = new Set(["Check", "Summary"]);

async function listJobs(github, context) {
  const response = await github.rest.actions.listJobsForWorkflowRun({
    owner: context.repo.owner,
    repo: context.repo.repo,
    run_id: context.runId,
    per_page: 100,
  });
  return response.data.jobs.filter((job) => !SKIP_JOBS.has(job.name));
}

function classifyJobs(jobs) {
  const results = { passed: [], failed: [], rateLimited: [], cancelled: [] };

  for (const job of jobs) {
    const name = job.name;

    if (job.conclusion === "success") {
      results.passed.push(name);
    } else if (job.conclusion === "cancelled") {
      results.cancelled.push(name);
    } else if (isRateLimited(job)) {
      results.rateLimited.push(name);
    } else {
      results.failed.push(name);
    }
  }

  return results;
}

function isRateLimited(job) {
  const marker = job.steps?.find((s) => s.name === "Rate limit marker");
  return marker?.conclusion === "success";
}

function formatSummary({ passed, failed, rateLimited, cancelled }) {
  const total = passed.length + failed.length + rateLimited.length + cancelled.length;
  let md = `## Acceptance Stage Results\n\n**${passed.length}/${total} passed**\n\n`;

  md += formatSection("Failed", failed);
  md += formatSection("Rate Limited (not a real failure)", rateLimited);
  md += formatSection("Cancelled", cancelled);
  md += formatSection("Passed", passed);

  return md;
}

function formatSection(title, items) {
  if (items.length === 0) return "";
  const list = items.map((n) => `- ${n}`).join("\n");
  return `### ${title}\n${list}\n\n`;
}
