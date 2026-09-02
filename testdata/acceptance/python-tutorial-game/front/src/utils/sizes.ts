function getCanvasHeight(windowsHeight: number) {
  return windowsHeight - 32 - 64 - 64.5 - 32;
}

function getCanvasWidth(windowsWidth: number) {
  return windowsWidth > 992
    ? Math.floor((windowsWidth - 30) * 0.62)
    : windowsWidth - 20;
}

export { getCanvasWidth, getCanvasHeight };
