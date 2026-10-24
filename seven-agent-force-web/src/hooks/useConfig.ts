import SystemSetting from "config/config";
import LayoutSetting from "config/layout";

const useConfig = () => {

  return {
    system: SystemSetting,
    layout: LayoutSetting
  }
}

export default useConfig;