(() => {
  const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const iconRules = [
    [/刷新|重新查询/, "refresh-cw"], [/退出/, "log-out"], [/登录/, "log-in"],
    [/新增|创建/, "plus"], [/生成/, "sparkles"], [/查询|筛选|搜索/, "search"],
    [/导出|下载/, "download"], [/查看|详情|明细|绑定/, "eye"], [/公钥|密钥/, "key-round"],
    [/轮换/, "rotate-cw"], [/恢复|启用/, "play"], [/停用|暂停/, "circle-pause"],
    [/吊销|作废|移除/, "ban"], [/解绑/, "unlink"], [/保存|确认/, "check"],
    [/激活/, "zap"], [/验证/, "badge-check"], [/上一页/, "arrow-left"], [/下一页/, "arrow-right"],
    [/全部/, "arrow-up-right"], [/测试数据/, "flask-conical"], [/账号/, "user-round"],
    [/授权规则|政策/, "sliders-horizontal"], [/额度/, "gauge"], [/卡密/, "ticket-check"],
  ];
  const getIcon = (element) => element.dataset.icon || iconRules.find(([pattern]) => pattern.test(element.textContent.trim()))?.[1] || "arrow-up-right";
  let refreshQueued = false;

  function refreshIcons(root = document) {
    if (!window.lucide) return;
    root.querySelectorAll("button:not([data-no-icon]), a[data-icon], .icon-label[data-icon]").forEach((element) => {
      if (element.querySelector(":scope > [data-lucide], :scope > svg.lucide")) return;
      const icon = document.createElement("i");
      icon.dataset.lucide = getIcon(element);
      icon.setAttribute("aria-hidden", "true");
      element.prepend(icon);
    });
    window.lucide.createIcons({ attrs: { "stroke-width": 1.8 } });
  }

  function queueIconRefresh() {
    if (refreshQueued) return;
    refreshQueued = true;
    requestAnimationFrame(() => {
      refreshQueued = false;
      refreshIcons();
    });
  }

  function setupPointerField() {
    if (reduceMotion || !window.matchMedia("(pointer: fine)").matches) return;
    window.addEventListener("pointermove", (event) => {
      document.documentElement.style.setProperty("--pointer-x", `${event.clientX}px`);
      document.documentElement.style.setProperty("--pointer-y", `${event.clientY}px`);
    }, { passive: true });
  }

  function setupSignalCanvas() {
    const canvas = document.querySelector("#signalCanvas");
    if (!canvas || reduceMotion) return;
    const context = canvas.getContext("2d", { alpha: true });
    const pointer = { x: -1000, y: -1000 };
    let width = 0;
    let height = 0;
    let frame = 0;
    let nodes = [];
    function resize() {
      const ratio = Math.min(window.devicePixelRatio || 1, 2);
      width = canvas.clientWidth;
      height = canvas.clientHeight;
      canvas.width = width * ratio;
      canvas.height = height * ratio;
      context.setTransform(ratio, 0, 0, ratio, 0, 0);
      const count = Math.max(26, Math.floor((width * height) / 26000));
      nodes = Array.from({ length: count }, (_, index) => ({ x: Math.random() * width, y: Math.random() * height, vx: (Math.random() - 0.5) * 0.22, vy: (Math.random() - 0.5) * 0.22, phase: index * 0.72 }));
    }
    function draw(time) {
      context.clearRect(0, 0, width, height);
      for (const node of nodes) {
        node.x += node.vx;
        node.y += node.vy;
        if (node.x < 0 || node.x > width) node.vx *= -1;
        if (node.y < 0 || node.y > height) node.vy *= -1;
        const dx = pointer.x - node.x;
        const dy = pointer.y - node.y;
        if (Math.hypot(dx, dy) < 180) { node.x -= dx * 0.0018; node.y -= dy * 0.0018; }
      }
      for (let i = 0; i < nodes.length; i += 1) {
        const node = nodes[i];
        for (let j = i + 1; j < nodes.length; j += 1) {
          const other = nodes[j];
          const distance = Math.hypot(node.x - other.x, node.y - other.y);
          if (distance > 145) continue;
          context.strokeStyle = `rgba(135, 255, 190, ${0.12 * (1 - distance / 145)})`;
          context.lineWidth = 0.7;
          context.beginPath(); context.moveTo(node.x, node.y); context.lineTo(other.x, other.y); context.stroke();
        }
        const pulse = 1.4 + Math.sin(time * 0.001 + node.phase) * 0.7;
        context.fillStyle = "rgba(182, 255, 93, 0.64)";
        context.beginPath(); context.arc(node.x, node.y, pulse, 0, Math.PI * 2); context.fill();
      }
      frame = requestAnimationFrame(draw);
    }
    canvas.addEventListener("pointermove", (event) => { const rect = canvas.getBoundingClientRect(); pointer.x = event.clientX - rect.left; pointer.y = event.clientY - rect.top; });
    canvas.addEventListener("pointerleave", () => Object.assign(pointer, { x: -1000, y: -1000 }));
    window.addEventListener("resize", resize, { passive: true });
    document.addEventListener("visibilitychange", () => { cancelAnimationFrame(frame); if (!document.hidden) frame = requestAnimationFrame(draw); });
    resize(); frame = requestAnimationFrame(draw);
  }

  function setupReveal() {
    const elements = document.querySelectorAll("[data-reveal]");
    if (reduceMotion) { elements.forEach((element) => element.classList.add("revealed")); return; }
    const observer = new IntersectionObserver((entries) => entries.forEach((entry) => {
      if (entry.isIntersecting) { entry.target.classList.add("revealed"); observer.unobserve(entry.target); }
    }), { threshold: 0.12 });
    elements.forEach((element) => observer.observe(element));
  }

  document.addEventListener("DOMContentLoaded", () => {
    refreshIcons(); setupPointerField(); setupSignalCanvas(); setupReveal();
    new MutationObserver(queueIconRefresh).observe(document.body, { childList: true, subtree: true });
  });
  window.YNUI = { refreshIcons, queueIconRefresh };
})();
