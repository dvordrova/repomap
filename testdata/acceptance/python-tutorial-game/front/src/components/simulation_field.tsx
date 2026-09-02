import React, { useRef, useState, useCallback, useEffect } from "react";
import styled from "styled-components";

import { debounce, defer } from "lodash";
import { Button, Slider, Spin, Flex, Radio, Typography } from "antd";

import { PauseOutlined, CaretRightFilled } from "@ant-design/icons";

import { ILevel, ISimulationStep } from "../utils/model";
import { LevelDrawer } from "../utils/draw";

const { Title } = Typography;

// display: block;
// overflow: auto;
const SimulationCanvas = styled.canvas`
  @media (max-width: 992px) {
    max-width: calc(100vw - 20px);
    height: 100vh;
    max-height: 100vh;
  }

  @media (min-width: 992px) {
    max-width: calc(62vw - 0.62 * 30px);
  }
`;

const PlaySliderWrapper = styled.div`
  display: flex;
  align-items: center;
  width: 100%;
  margin-top: 4px;
`;

const PlaySlider = styled(Slider)`
  flex-grow: 1;
`;

const PlayButton = styled(Button)`
  display: inline;
  margin-right: 10px;
`;

const SlownessRadioGroup = styled(Radio.Group)`
  margin-bottom: 16px;
`;

const SpeedTitle = styled(Title)`
  display: inline;
  margin-right: 16px;
`;

interface ISimulationFieldProps {
  level?: ILevel;
  simulationSteps?: ISimulationStep[];
  toggleSimulation: boolean;
}

const slownessOptions = [
  { label: "x1", value: 0.03 },
  { label: "x2", value: 0.06 },
  { label: "x5", value: 0.15 },
];

const devicePixelRatio =
  typeof window !== "undefined" ? window.devicePixelRatio || 1 : 1;

export default function SimulationField({
  level,
  simulationSteps,
  toggleSimulation,
}: ISimulationFieldProps) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [sliderRange, setSliderRange] = useState(0);
  const [runningSimulation, setRunningSimulation] = useState(false);
  const [simulationFrameStart, setSimulationFrameStart] = useState(0);
  const [slowness, setSlowness] = useState(0.03);
  const [sliderValue, setSliderValue] = useState(0);
  const [levelDrawer, setLevelDrawer] = useState<LevelDrawer>();

  // animation related
  const requestId = useRef<number>();
  const slownessRef = useRef<number>(slowness);
  const runningSimulationRef = useRef<boolean>(runningSimulation);
  const simulationStepsRef = useRef<ISimulationStep[] | undefined>(
    simulationSteps,
  );
  const levelDrawerRef = useRef<LevelDrawer | undefined>(levelDrawer);
  const levelRef = useRef<ILevel | undefined>(level);
  const simulationFrameStartRef = useRef<number>(simulationFrameStart);
  const simulationStepRef = useRef<number>(0);
  const simulationStepPercentageRef = useRef<number>(0.0);

  // level drawer setters
  useEffect(() => {
    setLevelDrawer(new LevelDrawer(canvasRef, level));
    if (toggleSimulation) {
      setRunningSimulation(false);
      simulationStepPercentageRef.current = 0;
      simulationStepRef.current = 0;
      setSliderValue(0);
    }
  }, [canvasRef, level, toggleSimulation]);

  // scale canvas
  useEffect(() => {
    const ctx = canvasRef?.current?.getContext("2d");
    if (ctx) {
      console.log("scale", { canvas: canvasRef.current, devicePixelRatio });
      ctx.scale(devicePixelRatio, devicePixelRatio);
      levelDrawer?.updateCanvasSize();
    }
  }, [canvasRef, levelDrawer]);
  const updateSize = debounce(() => {
    if (levelDrawer !== undefined) {
      levelDrawer.updateCanvasSize();
    }
  }, 300);
  useEffect(() => {
    setSliderRange(simulationSteps ? simulationSteps.length - 1 : 0);
    setSimulationFrameStart(0);
    simulationFrameStartRef.current = 0;
    setRunningSimulation(simulationSteps !== undefined);
  }, [simulationSteps, level, toggleSimulation]);

  useEffect(() => {
    window.addEventListener("resize", updateSize);
    return () => window.removeEventListener("resize", updateSize);
  });

  useEffect(() => {
    runningSimulationRef.current = runningSimulation;
    simulationStepsRef.current = simulationSteps;
    levelDrawerRef.current = levelDrawer;
    levelRef.current = level;
  }, [runningSimulation, simulationSteps, levelDrawer, level]);

  const animate = (_: any) => {
    defer(() => {
      requestId.current = requestAnimationFrame(animate);
    });
    if (!runningSimulationRef.current || !simulationStepsRef.current) {
      return;
    }
    simulationStepPercentageRef.current += slownessRef.current;
    if (simulationStepPercentageRef.current >= 1) {
      simulationStepPercentageRef.current -= 1;
      simulationStepRef.current++;
    }
    let simulationStep = simulationStepRef.current;
    let percentOfStep = simulationStepPercentageRef.current;
    if (simulationStep >= simulationStepsRef.current.length) {
      return;
    }
    setSliderValue(simulationStep);

    if (!levelDrawerRef.current) {
      return;
    }
    levelDrawerRef.current.drawLevel();
    if (levelRef.current?.walls) {
      levelDrawerRef.current.drawWalls(levelRef.current.walls);
    }

    // count of robots should be constant between steps
    let currentStep = simulationStepsRef.current[simulationStep];
    let nextStep =
      simulationStepsRef.current[
        Math.min(simulationStep + 1, simulationStepsRef.current.length - 1)
      ];

    console.log("drawing robots", {
      currentStep,
      nextStep,
      percentOfStep,
    });
    levelDrawerRef.current.drawRobotsBetweenSteps(
      currentStep.robots,
      nextStep.robots,
      percentOfStep,
    );
  };

  useEffect(() => {
    requestId.current = requestAnimationFrame(animate);
    return () => {
      if (requestId.current) {
        cancelAnimationFrame(requestId.current);
      }
    };
  }, []);

  const clickSlider = debounce((value) => {
    setSliderValue(value);
    simulationStepRef.current = value;
    simulationStepPercentageRef.current = 0;
    setRunningSimulation(true);
  }, 300);

  const onSlownessChange = (e: any) => {
    setSlowness(e.target.value);
    slownessRef.current = e.target.value;
  };

  const onPlayClick = () => {
    setRunningSimulation((value) => !value);
  };

  const playIcon = runningSimulation ? <PauseOutlined /> : <CaretRightFilled />;

  return (
    <>
      <PlaySliderWrapper>
        <PlayButton icon={playIcon} size="small" onClick={onPlayClick} />
        <PlaySlider
          value={sliderValue}
          disabled={!simulationSteps}
          max={sliderRange}
          onChange={clickSlider}
          tooltip={{ open: false }}
        />
      </PlaySliderWrapper>
      <SpeedTitle level={4}>Скорость</SpeedTitle>
      <SlownessRadioGroup
        options={slownessOptions}
        onChange={onSlownessChange}
        value={slowness}
        optionType="button"
      />
      <Flex justify="center">
        <Spin tip="Loading..." spinning={level === undefined}>
          <SimulationCanvas ref={canvasRef} />
        </Spin>
      </Flex>
    </>
  );
}
