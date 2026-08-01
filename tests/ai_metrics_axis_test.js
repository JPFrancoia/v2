const assert = require("node:assert/strict");
const fs = require("node:fs");
const test = require("node:test");

const app = fs.readFileSync("internal/ui/static/js/app.js", "utf8");
const start = app.indexOf("function calculateAIMetricsYAxis");
const end = app.indexOf("\nfunction renderAIMetricsChart", start);
const calculateAIMetricsYAxis = Function(
    `${app.slice(start, end)}; return calculateAIMetricsYAxis;`,
)();
const seriesStart = app.indexOf("function aiMetricsChartSeries");
const seriesEnd = app.indexOf("\nfunction renderAIMetricsChart", seriesStart);
const aiMetricsChartSeries = Function(
    `${app.slice(seriesStart, seriesEnd)}; return aiMetricsChartSeries;`,
)();
const pointValue = (point, key) => point[key] ?? null;

test("selects the Super-important chart series", () => {
    const series = aiMetricsChartSeries({ dataset: { isSuperImportant: "true" } });

    assert.deepEqual(series.map((item) => item.key), [
        "super_important_average_precision",
        "relevance_average_precision",
        "recall_at_50",
    ]);
});

test("zooms binary metrics near their data", () => {
    const axis = calculateAIMetricsYAxis(
        [{ f1: 0.90 }, { f1: 0.95 }],
        [{ key: "f1" }],
        pointValue,
        false,
    );

    assert.deepEqual(axis, {
        minimum: 0.8,
        maximum: 1,
        ticks: [0.8, 0.85, 0.9, 0.95, 1],
    });
});

test("includes negative Freshness metrics without forcing the full range", () => {
    const axis = calculateAIMetricsYAxis(
        [{ f1: 0.2, weighted_kappa: -0.3 }, { f1: 0.4, weighted_kappa: null }],
        [{ key: "f1" }, { key: "weighted_kappa" }],
        pointValue,
        true,
    );

    assert.equal(axis.minimum, -0.5);
    assert.equal(axis.maximum, 0.5);
});

test("uses the full valid range when no visible value exists", () => {
    assert.deepEqual(calculateAIMetricsYAxis([], [], pointValue, false), {
        minimum: 0,
        maximum: 1,
        ticks: [0, 0.25, 0.5, 0.75, 1],
    });
    assert.deepEqual(
        calculateAIMetricsYAxis([{ weighted_kappa: null }], [{ key: "weighted_kappa" }], pointValue, true),
        { minimum: -1, maximum: 1, ticks: [-1, -0.5, 0, 0.5, 1] },
    );
});

test("keeps a boundary value visible", () => {
    const axis = calculateAIMetricsYAxis([{ f1: 1 }], [{ key: "f1" }], pointValue, false);

    assert.equal(axis.maximum, 1);
    assert.ok(axis.minimum < 1);
});
