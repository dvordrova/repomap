import { awardColor, cellColor, fieldColor, gapBeetween } from "./constants";
import { ILevel, IRobot, IWall } from "./model";

import { getCanvasWidth, getCanvasHeight } from "./sizes";

import robotImage from "../assets/vacuum-cleaner-robot.png";

const topSensorY = 199;
const bottomSensorY = 800;
const leftSensorX = 204;
const rightSensorX = 868;

const devicePixelRatio =
  typeof window !== "undefined" ? window.devicePixelRatio || 1 : 1;

class LevelDrawer {
  ctx?: CanvasRenderingContext2D | null;
  robotImage: HTMLImageElement;
  level?: ILevel;
  width: number = 0;
  height: number = 0;
  cellSize: number = 0;

  constructor(canvasRef: React.RefObject<HTMLCanvasElement>, level?: ILevel) {
    this.ctx = canvasRef?.current?.getContext("2d");
    this.robotImage = new Image();
    this.robotImage.src = robotImage;
    this.robotImage.onload = () => {
      this.updateCanvasSize();
      // this.redraw();
    };
    this.level = level;
    this.updateCanvasSize();
  }

  updateCellSize() {
    if (!this.level) {
      return;
    }
    this.cellSize = Math.min(
      (this.width - gapBeetween) / this.level.width - gapBeetween,
      (this.height - gapBeetween) / this.level.height - gapBeetween,
    );
  }

  updateCanvasSize() {
    this.width = getCanvasWidth(window.innerWidth);
    this.height = getCanvasHeight(window.innerHeight);
    if (this.level) {
      if (this.level.width / this.level.height < this.width / this.height) {
        this.width = this.height * (this.level.width / this.level.height);
      } else {
        this.height = this.width * (this.level.height / this.level.width);
      }
    }
    if (!this.ctx) {
      return;
    }
    this.ctx.canvas.style.width = `${this.width}px`;
    this.ctx.canvas.style.height = `${this.height}px`;
    this.ctx.canvas.width = this.width * devicePixelRatio;
    this.ctx.canvas.height = this.height * devicePixelRatio;

    console.log("style", {
      style_width: this.width,
      style_height: this.height,
      width: this.width * devicePixelRatio,
      height: this.height * devicePixelRatio,
    });

    this.ctx.scale(devicePixelRatio, devicePixelRatio);

    this.updateCellSize();
    this.redraw();
  }

  redraw() {
    this.drawLevel();
    if (this.level) {
      this.drawRobots(this.level.robots);
      this.drawWalls(this.level.walls);
    }
  }

  setLevel(level?: ILevel) {
    this.level = level;
    this.updateCanvasSize();
  }

  drawAward(centerX: number, centerY: number) {
    if (!this.ctx) {
      return;
    }
    let size = this.cellSize / 2;
    this.ctx.beginPath();
    this.ctx.moveTo(centerX + Math.cos(0) * size, centerY + Math.sin(0) * size);
    for (let i = 1; i <= 5; ++i) {
      this.ctx.lineTo(
        centerX + (Math.cos((i * 2 * Math.PI) / 5 - Math.PI / 5) * size) / 2,
        centerY + (Math.sin((i * 2 * Math.PI) / 5 - Math.PI / 5) * size) / 2,
      );
      this.ctx.lineTo(
        centerX + Math.cos((i * 2 * Math.PI) / 5) * size,
        centerY + Math.sin((i * 2 * Math.PI) / 5) * size,
      );
    }
    this.ctx.closePath();
    this.ctx.fill();
  }

  drawLevel() {
    if (!this.ctx || !this.level) {
      console.log("no ctx or level");
      return;
    }
    this.ctx.clearRect(0, 0, this.width, this.height);
    this.ctx.fillStyle = fieldColor;
    this.ctx.fillRect(
      0,
      0,
      (this.cellSize + gapBeetween) * this.level.width + gapBeetween,
      (this.cellSize + gapBeetween) * this.level.height + gapBeetween,
    );
    this.ctx.fillStyle = cellColor;
    this.ctx.beginPath();
    for (let x = 0; x < this.level.width; x++) {
      for (let y = 0; y < this.level.height; y++) {
        this.ctx.rect(
          gapBeetween + x * (this.cellSize + gapBeetween),
          gapBeetween + y * (this.cellSize + gapBeetween),
          this.cellSize,
          this.cellSize,
        );
      }
    }
    console.debug("drawLevel fill");
    this.ctx.fill();
    this.ctx.fillStyle = awardColor;
    for (let i = 0; i < this.level.awards.length; i++) {
      console.debug("drawLevel award", i);
      let award = this.level.awards[i];
      this.drawAward(
        gapBeetween +
          award.x * (this.cellSize + gapBeetween) +
          this.cellSize / 2,
        gapBeetween +
          award.y * (this.cellSize + gapBeetween) +
          this.cellSize / 2,
      );
    }
  }

  drawSensor(x: number, y: number) {
    if (!this.ctx) {
      return;
    }
    let radius = (this.cellSize * 22) / this.robotImage.width;
    for (let i = 1; i <= 3; i++) {
      let opacity = 0.5;
      this.ctx.fillStyle = "rgba(200, 0, 0, " + opacity + ")";
      this.ctx.beginPath();
      this.ctx.arc(x, y, radius * 2 ** i, 0, 2 * Math.PI);
      this.ctx.closePath();
      this.ctx.fill();
    }
  }

  drawSensorUp(left_x: number, top_y: number) {
    this.drawSensor(
      left_x +
        (this.cellSize * (leftSensorX + rightSensorX)) /
          2 /
          this.robotImage.width,
      top_y + (this.cellSize * topSensorY) / this.robotImage.width,
    );
  }

  drawSensorRight(left_x: number, top_y: number) {
    this.drawSensor(
      left_x + (this.cellSize * rightSensorX) / this.robotImage.width,
      top_y +
        (this.cellSize * (topSensorY + bottomSensorY)) /
          2 /
          this.robotImage.width,
    );
  }

  drawSensorDown(left_x: number, top_y: number) {
    this.drawSensor(
      left_x +
        (this.cellSize * (leftSensorX + rightSensorX)) /
          2 /
          this.robotImage.width,
      top_y + (this.cellSize * bottomSensorY) / this.robotImage.width,
    );
  }

  drawSensorLeft(left_x: number, top_y: number) {
    this.drawSensor(
      left_x + (this.cellSize * leftSensorX) / this.robotImage.width,
      top_y +
        (this.cellSize * (topSensorY + bottomSensorY)) /
          2 /
          this.robotImage.width,
    );
  }

  drawRobot(robot: IRobot) {
    if (!this.ctx) {
      return;
    }
    this.ctx.drawImage(
      this.robotImage,
      robot.x,
      robot.y,
      this.cellSize,
      this.cellSize,
    );
    if (robot.sensors.up) {
      this.drawSensorUp(robot.x, robot.y);
    }
    if (robot.sensors.right) {
      this.drawSensorRight(robot.x, robot.y);
    }
    if (robot.sensors.down) {
      this.drawSensorDown(robot.x, robot.y);
    }
    if (robot.sensors.left) {
      this.drawSensorLeft(robot.x, robot.y);
    }
  }

  drawRobots(robots: IRobot[]) {
    if (!this.ctx) {
      console.debug("drawRobots no ctx");
      return;
    }
    console.debug("drawRobots");
    for (let i = 0; i < robots.length; i++) {
      let x = gapBeetween + robots[i].x * (this.cellSize + gapBeetween);
      let y = gapBeetween + robots[i].y * (this.cellSize + gapBeetween);
      this.drawRobot({ x, y, sensors: robots[i].sensors });
    }
  }

  drawWalls(walls: IWall[]) {
    if (!this.ctx) {
      console.debug("drawWalls no ctx");
      return;
    }
    console.debug("drawWalls");
    this.ctx.strokeStyle = "#ff0000"; // Line color
    this.ctx.lineWidth = 5; // Line width
    for (let i = 0; i < walls.length; i++) {
      if (walls[i].type === "horizontal") {
        this.ctx.beginPath();
        this.ctx.moveTo(
          walls[i].x * (this.cellSize + gapBeetween),
          walls[i].y * (this.cellSize + gapBeetween),
        );
        this.ctx.lineTo(
          (walls[i].x + 1) * (this.cellSize + gapBeetween),
          walls[i].y * (this.cellSize + gapBeetween),
        );
        this.ctx.stroke();
      } else if (walls[i].type === "vertical") {
        this.ctx.beginPath();
        this.ctx.moveTo(
          walls[i].x * (this.cellSize + gapBeetween),
          walls[i].y * (this.cellSize + gapBeetween),
        );
        this.ctx.lineTo(
          walls[i].x * (this.cellSize + gapBeetween),
          (walls[i].y + 1) * (this.cellSize + gapBeetween),
        );
        this.ctx.stroke();
      } else {
        console.debug("drawWalls unknown wall type");
      }
    }
  }

  drawRobotsBetweenSteps(
    robots: IRobot[],
    next_robots: IRobot[],
    percentOfStep: number,
  ) {
    if (!this.ctx) {
      return;
    }
    for (let i = 0; i < robots.length; i++) {
      let cur_robot = robots[i];
      let next_robot = next_robots[i];
      let x =
        cur_robot.x * (this.cellSize + gapBeetween) * (1 - percentOfStep) +
        next_robot.x * (this.cellSize + gapBeetween) * percentOfStep;
      let y =
        cur_robot.y * (this.cellSize + gapBeetween) * (1 - percentOfStep) +
        next_robot.y * (this.cellSize + gapBeetween) * percentOfStep;
      let sensors = {
        up:
          (cur_robot.sensors.up && percentOfStep < 0.1) ||
          next_robot.sensors.up,
        right:
          (cur_robot.sensors.right && percentOfStep < 0.1) ||
          next_robot.sensors.right,
        left:
          (cur_robot.sensors.left && percentOfStep < 0.1) ||
          next_robot.sensors.left,
        down:
          (cur_robot.sensors.down && percentOfStep < 0.1) ||
          next_robot.sensors.down,
      };
      this.drawRobot({ x, y, sensors });
    }
  }
}

export { LevelDrawer };
