"""Produce exact compact CTC scores on CUDA with FP32 Softmax / TopK / Gather; never overwrite source models."""
import argparse
from pathlib import Path
import onnx
from onnx import helper, TensorProto

parser = argparse.ArgumentParser()
parser.add_argument("source")
parser.add_argument("destination")
args = parser.parse_args()
source = Path(args.source).resolve()
destination = Path(args.destination).resolve()
if source == destination:
    raise SystemExit("destination must differ from source")
model = onnx.load(str(source))
if len(model.graph.output) != 1 or model.graph.output[0].name != "logits":
    raise SystemExit("expected a single logits output")
model.graph.node.append(helper.make_node("Softmax", ["logits"], ["probabilities"], axis=-1, name="wxocr_output_softmax"))
# TopK breaks equal values by the lowest class index, matching Go greedy CTC.
for name,values in [("wx_axis0",[0]),("wx_axis2",[2]),("wx_blank",[0]),("wx_k1",[1])]:
    model.graph.initializer.append(helper.make_tensor(name, TensorProto.INT64, [1], values))
model.graph.node.extend([
    helper.make_node("TopK", ["probabilities","wx_k1"], ["wx_top_values","wx_top_indices"], axis=2, largest=1, sorted=1, name="wxocr_ctc_top1"),
    helper.make_node("Squeeze", ["wx_top_indices","wx_axis2"], ["wx_greedy"]),
    helper.make_node("Squeeze", ["wx_greedy","wx_axis0"], ["wx_indices"]),
    helper.make_node("Gather", ["probabilities","wx_indices"], ["wx_selected"], axis=2),
    helper.make_node("Gather", ["probabilities","wx_blank"], ["wx_blank_prob"], axis=2),
    helper.make_node("Cast", ["wx_greedy"], ["wx_ids_float"], to=TensorProto.FLOAT),
    helper.make_node("Unsqueeze", ["wx_ids_float","wx_axis2"], ["wx_ids_column"]),
    helper.make_node("Concat", ["wx_ids_column","wx_blank_prob","wx_selected"], ["ctc_compact_v1"], axis=2),
])
del model.graph.output[:]
model.graph.output.append(helper.make_tensor_value_info("ctc_compact_v1", TensorProto.FLOAT, [1,"steps","selected_classes_plus_two"]))
onnx.checker.check_model(model)
destination.parent.mkdir(parents=True, exist_ok=True)
onnx.save(model, str(destination))
