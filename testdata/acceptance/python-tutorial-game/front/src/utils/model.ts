interface IGetLevelsResponse {
  count: number;
}

enum RequestState {
  Error = "error",
  Success = "success",
}
enum WallType {
  Vertical = "vertical",
  Horizontal = "horizontal",
}

interface IPoint {
  x: number;
  y: number;
}

interface IWall {
  x: number;
  y: number;
  type: WallType;
}

interface ILevel {
  id: number;
  width: number;
  height: number;
  robots: IRobot[];
  awards: IPoint[];
  walls: IWall[];
}

interface ILevelDescription {
  state: RequestState;
  level?: ILevel;
  reason?: string;
}

interface IRunLevelRequest {
  level_id: number;
  code: string;
}

interface ISensors {
  up: boolean;
  down: boolean;
  left: boolean;
  right: boolean;
}

interface IRobot {
  x: number;
  y: number;
  sensors: ISensors;
}

interface ISimulationStep {
  robots: IRobot[];
}

enum SimulationRunResult {
  Win = "win",
  Lose = "lose",
}

interface IRunDescription {
  state: RequestState;
  steps?: ISimulationStep[];
  result?: SimulationRunResult;
  reason?: string;
}

export type {
  IGetLevelsResponse,
  IRunDescription,
  ISimulationStep,
  IRunLevelRequest,
  ILevelDescription,
  ILevel,
  IRobot,
  IPoint,
  IWall,
};

export { SimulationRunResult, RequestState };
