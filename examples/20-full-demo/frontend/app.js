// Zeus cluster routing demo 前端逻辑
//
// 调用链：gateway → api-1 → srv-1 → srv-2 → srv-3
// 4 个入口 cluster（default / user.v1.1 / order.v2 / batch.v3），按部署矩阵：
//
//   cluster    | api1 | srv1 | srv2 | srv3
//   -----------|------|------|------|------
//   default    |  ✓   |  ✓   |  ✓   |  ✓
//   user.v1.1  |      |  ✓   |  ✓   |
//   order.v2   |      |      |  ✓   |
//   batch.v3   |  ✓   |  ✓   |      |  ✓
//
// 缺失节点自动降级到 default（zeus client 内置逻辑）

const API_BASE = window.location.origin.startsWith('http://localhost')
  ? 'http://localhost:8080'
  : '';
const TOPOLOGY_SVG = document.getElementById('topology');
const TRACE_OUTPUT = document.getElementById('trace-output');

// === 布局常量 ===
// 左侧集群标签独立列（0~110），节点区域从 130 开始
const LABEL_X = 8;           // 集群标签文字 x
const ROW_H = 76;            // 每行高度
const ROW_GAP = 8;           // 行间距
const ROW_START_Y = 50;      // 第一行顶部
const NODE_W = 110;
const NODE_H = 48;
const COL_X = { gateway: 130, api1: 280, srv1: 430, srv2: 580, srv3: 730 };
const GATEWAY_X = 130;

// 集群行信息（动态计算 y 坐标）
const CLUSTERS = ['default', 'user.v1.1', 'order.v2', 'batch.v3'];
const ROW_Y = {};
CLUSTERS.forEach((c, i) => {
  ROW_Y[c] = ROW_START_Y + i * (ROW_H + ROW_GAP) + ROW_H / 2;
});

// 集群颜色 + 显示名
const CLUSTER_META = {
  'default':   { color: '#4a90e2', label: 'default' },
  'user.v1.1': { color: '#e67e22', label: 'user.v1.1' },
  'order.v2':  { color: '#16a085', label: 'order.v2' },
  'batch.v3':  { color: '#9b59b6', label: 'batch.v3' },
};
const FALLBACK_COLOR = '#95a5a6';

// 部署矩阵：service → Set(clusters)
const DEPLOY_MATRIX = {
  api1: ['default', 'batch.v3'],
  srv1: ['default', 'user.v1.1', 'batch.v3'],
  srv2: ['default', 'user.v1.1', 'order.v2'],
  srv3: ['default', 'batch.v3'],
};

// 每个入口 cluster 对应的流量路径
const PATHS = {
  'default': [
    { from: 'gateway', fromCluster: '',         to: 'api1', toCluster: 'default' },
    { from: 'api1',    fromCluster: 'default',  to: 'srv1', toCluster: 'default' },
    { from: 'srv1',    fromCluster: 'default',  to: 'srv2', toCluster: 'default' },
    { from: 'srv2',    fromCluster: 'default',  to: 'srv3', toCluster: 'default' },
  ],
  'user.v1.1': [
    { from: 'gateway', fromCluster: '',         to: 'api1', toCluster: 'default',   fallback: true },
    { from: 'api1',    fromCluster: 'default',  to: 'srv1', toCluster: 'user.v1.1' },
    { from: 'srv1',    fromCluster: 'user.v1.1',to: 'srv2', toCluster: 'user.v1.1' },
    { from: 'srv2',    fromCluster: 'user.v1.1',to: 'srv3', toCluster: 'default',   fallback: true },
  ],
  'order.v2': [
    { from: 'gateway', fromCluster: '',         to: 'api1', toCluster: 'default', fallback: true },
    { from: 'api1',    fromCluster: 'default',  to: 'srv1', toCluster: 'default', fallback: true },
    { from: 'srv1',    fromCluster: 'default',  to: 'srv2', toCluster: 'order.v2' },
    { from: 'srv2',    fromCluster: 'order.v2', to: 'srv3', toCluster: 'default', fallback: true },
  ],
  'batch.v3': [
    { from: 'gateway', fromCluster: '',         to: 'api1', toCluster: 'batch.v3' },
    { from: 'api1',    fromCluster: 'batch.v3', to: 'srv1', toCluster: 'batch.v3' },
    { from: 'srv1',    fromCluster: 'batch.v3', to: 'srv2', toCluster: 'default', fallback: true },
    { from: 'srv2',    fromCluster: 'default',  to: 'srv3', toCluster: 'batch.v3' },
  ],
};

let activeCluster = null;

async function init() {
  await refreshTopology();
  document.getElementById('btn-default').onclick   = () => sendRequest('default');
  document.getElementById('btn-user').onclick      = () => sendRequest('user.v1.1');
  document.getElementById('btn-order').onclick     = () => sendRequest('order.v2');
  document.getElementById('btn-batch').onclick     = () => sendRequest('batch.v3');
  document.getElementById('btn-refresh').onclick   = refreshTopology;
  if (document.getElementById('auto-refresh').checked) {
    setInterval(refreshTopology, 2000);
  }
}

async function refreshTopology() {
  try {
    const resp = await fetch(`${API_BASE}/api/services`);
    const data = await resp.json();
    drawTopology(data.services || {});
  } catch (e) {
    console.error('refresh topology failed:', e);
  }
}

function drawTopology(services) {
  const svg = [];

  // 1. 集群行背景框 + 左侧标签
  CLUSTERS.forEach((cluster, i) => {
    const meta = CLUSTER_META[cluster];
    const rowTop = ROW_START_Y + i * (ROW_H + ROW_GAP);
    // 背景框（节点区域）
    svg.push(`<rect x="120" y="${rowTop}" width="730" height="${ROW_H}" rx="6"
      fill="${hexToRgba(meta.color, 0.04)}" stroke="${meta.color}" stroke-dasharray="4,3" opacity="0.7" />`);
    // 集群标签（左侧独立区域，带色块）
    svg.push(`<rect x="${LABEL_X}" y="${rowTop + 2}" width="100" height="${ROW_H - 4}" rx="4"
      fill="${hexToRgba(meta.color, 0.12)}" stroke="${meta.color}" stroke-width="1.5" />`);
    svg.push(`<text x="${LABEL_X + 50}" y="${rowTop + ROW_H / 2 + 5}" text-anchor="middle"
      font-size="12" font-weight="bold" fill="${meta.color}" font-family="monospace">${meta.label}</text>`);
  });

  // 2. Gateway 节点（跨集群，居中）
  const gwY = (ROW_Y['default'] + ROW_Y['batch.v3']) / 2;
  svg.push(node(COL_X.gateway, gwY - NODE_H / 2, 'Gateway', '入口', '#2c3e50', '#fff'));

  // 3. 服务节点（4 列 × 4 集群行；按部署矩阵渲染）
  ['api1', 'srv1', 'srv2', 'srv3'].forEach(svc => {
    CLUSTERS.forEach(cluster => {
      const deployed = DEPLOY_MATRIX[svc].includes(cluster);
      const meta = CLUSTER_META[cluster];
      const count = countCluster(services[svc], cluster);
      const sub = deployed
        ? (count > 0 ? `${count}实例` : '已部署')
        : '—';
      const fill = deployed ? meta.color : '#f0f0f0';
      const textColor = deployed ? '#fff' : '#ccc';
      svg.push(node(COL_X[svc], ROW_Y[cluster] - NODE_H / 2, svc, sub, fill, textColor, cluster, svc));
    });
  });

  // 4. 所有可能的边（按入口 cluster 渲染，去重）
  const drawnEdges = new Set();
  Object.values(PATHS).forEach(path => {
    path.forEach(hop => {
      const eid = edgeId(hop);
      if (drawnEdges.has(eid)) return;
      drawnEdges.add(eid);
      const color = hop.fallback ? FALLBACK_COLOR : CLUSTER_META[hop.toCluster].color;
      const isCross = hop.fromCluster !== hop.toCluster;
      const fromPos = hop.from === 'gateway'
        ? { x: COL_X.gateway + NODE_W, y: gwY }
        : { x: COL_X[hop.from] + NODE_W, y: ROW_Y[hop.fromCluster] };
      const toPos = { x: COL_X[hop.to], y: ROW_Y[hop.toCluster] };
      svg.push(arrow(fromPos, toPos, color, eid, hop.fallback || isCross));
    });
  });

  TOPOLOGY_SVG.innerHTML = svg.join('');
  applyHighlight();
}

function countCluster(instances, cluster) {
  if (!instances) return 0;
  return instances.filter(i => i.cluster === cluster).length;
}

function edgeId(hop) {
  return `${hop.from}-${hop.fromCluster || 'gw'}-${hop.to}-${hop.toCluster}`;
}

async function sendRequest(cluster) {
  activeCluster = cluster;
  applyHighlight();
  const btnId = ({
    'default':   'btn-default',
    'user.v1.1': 'btn-user',
    'order.v2':  'btn-order',
    'batch.v3':  'btn-batch',
  })[cluster];
  const btn = document.getElementById(btnId);
  btn.disabled = true;
  TRACE_OUTPUT.textContent = `→ 发送 /login 请求（cluster=${cluster}）...\n`;

  try {
    const resp = await fetch(`${API_BASE}/login`, {
      headers: { 'X-Zeus-Cluster': cluster },
    });
    const text = await resp.text();
    TRACE_OUTPUT.textContent += `← HTTP ${resp.status}\n\n`;
    TRACE_OUTPUT.textContent += formatTrace(JSON.parse(text), 0);
  } catch (e) {
    TRACE_OUTPUT.textContent += `错误：${e.message}\n\n提示：请确认 gateway 已启动、所有 srv 已注册。`;
  } finally {
    btn.disabled = false;
    setTimeout(refreshTopology, 500);
  }
}

// 格式化嵌套调用链
function formatTrace(node, depth) {
  const indent = '  '.repeat(depth);
  const arrow = depth === 0 ? '📌' : '↓';
  const isDifferentCluster = node.cluster !== activeCluster && node.cluster === 'default';
  const fallbackMark = isDifferentCluster && depth > 0 ? '  ⚠️ 降级 default' : '';
  const lines = [`${indent}${arrow} ${node.service} [${node.cluster}] ${node.version} → ${node.action}${fallbackMark}`];
  if (node.downstream) {
    if (typeof node.downstream === 'string') {
      lines.push(`${indent}   响应: ${node.downstream}`);
    } else {
      lines.push(formatTrace(node.downstream, depth + 1));
    }
  }
  return lines.join('\n');
}

// 高亮当前流量路径
function applyHighlight() {
  TOPOLOGY_SVG.querySelectorAll('.node').forEach(n => {
    n.classList.remove('node-active');
    n.classList.remove('node-fallback');
  });
  TOPOLOGY_SVG.querySelectorAll('.arrow').forEach(a => a.classList.remove('arrow-active'));
  TOPOLOGY_SVG.querySelectorAll('.row-label').forEach(r => r.classList.remove('row-active'));

  if (!activeCluster) return;
  const path = PATHS[activeCluster];
  if (!path) return;

  // 高亮集群行标签
  const rowLabel = TOPOLOGY_SVG.querySelector(`.row-label[data-cluster="${activeCluster}"]`);
  if (rowLabel) rowLabel.classList.add('row-active');

  // 高亮路径上的节点和边
  path.forEach(hop => {
    if (hop.from !== 'gateway') {
      highlightNode(hop.from, hop.fromCluster, hop.fallback && hop.fromCluster === 'default');
    }
    highlightNode(hop.to, hop.toCluster, hop.fallback);
    highlightArrow(edgeId(hop));
  });
}

function highlightNode(name, cluster, isFallback) {
  const sel = TOPOLOGY_SVG.querySelector(`.node[data-cluster="${cluster}"][data-name="${name}"]`);
  if (sel) {
    sel.classList.add('node-active');
    if (isFallback) sel.classList.add('node-fallback');
  }
}

function highlightArrow(edgeId) {
  const sel = TOPOLOGY_SVG.querySelector(`.arrow[data-edge="${edgeId}"]`);
  if (sel) sel.classList.add('arrow-active');
}

// SVG 元素构造工具
function node(x, y, name, sub, fill, textColor, cluster = '', dataName = '') {
  const cls = cluster ? `node node-${cluster.replace(/\./g, '-')}` : 'node';
  const attrs = cluster ? `data-cluster="${cluster}" data-name="${dataName}"` : '';
  return `<g class="${cls}" ${attrs} transform="translate(${x},${y})">
    <rect width="${NODE_W}" height="${NODE_H}" rx="6" fill="${fill}" stroke="#34495e" stroke-width="1.5" />
    <text x="${NODE_W / 2}" y="20" text-anchor="middle" font-size="13" font-weight="bold" fill="${textColor}">${name}</text>
    <text x="${NODE_W / 2}" y="36" text-anchor="middle" font-size="10" fill="${textColor}" opacity="0.85">${sub}</text>
  </g>`;
}

function arrow(from, to, color, edgeId, dashed) {
  const dash = dashed ? 'stroke-dasharray="6,4"' : '';
  return `<line class="arrow" data-edge="${edgeId}" x1="${from.x}" y1="${from.y}" x2="${to.x}" y2="${to.y}"
          stroke="${color}" stroke-width="2" marker-end="url(#ah-${CSS.escape(edgeId)})" opacity="0.35" ${dash} />
          <defs>
            <marker id="ah-${CSS.escape(edgeId)}" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
              <polygon points="0 0, 6 3, 0 6" fill="${color}" />
            </marker>
          </defs>`;
}

function hexToRgba(hex, alpha) {
  const h = hex.replace('#', '');
  const r = parseInt(h.substr(0, 2), 16);
  const g = parseInt(h.substr(2, 2), 16);
  const b = parseInt(h.substr(4, 2), 16);
  return `rgba(${r},${g},${b},${alpha})`;
}

init();
