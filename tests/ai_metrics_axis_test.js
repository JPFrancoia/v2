const assert = require("node:assert/strict");
const fs = require("node:fs");
const test = require("node:test");

const app = fs.readFileSync("internal/ui/static/js/app.js", "utf8");
const start = app.indexOf("function calculateAIMetricsYAxis");
const end = app.indexOf("\nfunction renderAIMetricsChart", start);
const calculateAIMetricsYAxis = Function(
    `${app.slice(start, end)}; return calculateAIMetricsYAxis;`,
)();
const valueCheckStart = app.indexOf("function hasAIMetricsChartValue");
const valueCheckEnd = app.indexOf("\nfunction aiMetricsChartSeries", valueCheckStart);
const hasAIMetricsChartValue = Function(
    `${app.slice(valueCheckStart, valueCheckEnd)}; return hasAIMetricsChartValue;`,
)();
const seriesStart = app.indexOf("function aiMetricsChartSeries");
const seriesEnd = app.indexOf("\nfunction renderAIMetricsChart", seriesStart);
const aiMetricsChartSeries = Function(
    `${app.slice(seriesStart, seriesEnd)}; return aiMetricsChartSeries;`,
)();
const pointValue = (point, key) => point[key] ?? null;

test("detects empty discovery chart data", () => {
    assert.equal(hasAIMetricsChartValue([{ date: "2026-08-03", rate: null }]), false);
    assert.equal(hasAIMetricsChartValue([{ date: "2026-08-03", rate: 0 }]), true);
});

test("selects the discovery rate chart series", () => {
    const series = aiMetricsChartSeries({ dataset: { isDiscovery: "true" } });

    assert.deepEqual(series.map((item) => item.key), ["rate"]);
});

test("selects the Relevance ranking chart series", () => {
    const series = aiMetricsChartSeries({ dataset: { isRelevance: "true" } });

    assert.deepEqual(series.map((item) => item.key), [
        "average_precision",
        "precision_at_50",
    ]);
});

test("zooms metrics near their data", () => {
    const axis = calculateAIMetricsYAxis(
        [{ f1: 0.90 }, { f1: 0.95 }],
        [{ key: "f1" }],
        pointValue,
    );

    assert.deepEqual(axis, {
        minimum: 0.8,
        maximum: 1,
        ticks: [0.8, 0.85, 0.9, 0.95, 1],
    });
});

test("uses the full valid range when no visible value exists", () => {
    assert.deepEqual(calculateAIMetricsYAxis([], [], pointValue), {
        minimum: 0,
        maximum: 1,
        ticks: [0, 0.25, 0.5, 0.75, 1],
    });
});
